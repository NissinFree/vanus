// Copyright 2022 Linkall Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package store

import (
	// standard libraries
	"context"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/linkall-labs/vanus/observability/tracing"
	"go.opentelemetry.io/otel/trace"

	// third-party libraries
	cepb "cloudevents.io/genproto/v1"
	ce "github.com/cloudevents/sdk-go/v2"
	"google.golang.org/grpc"

	// first-party libraries
	errpb "github.com/linkall-labs/vanus/proto/pkg/errors"
	segpb "github.com/linkall-labs/vanus/proto/pkg/segment"

	// this project
	"github.com/linkall-labs/vanus/client/internal/vanus/codec"
	"github.com/linkall-labs/vanus/client/internal/vanus/net/rpc"
	"github.com/linkall-labs/vanus/client/internal/vanus/net/rpc/bare"
	"github.com/linkall-labs/vanus/client/pkg/primitive"
	"github.com/linkall-labs/vanus/observability/log"
	"github.com/linkall-labs/vanus/pkg/errors"
)

func newBlockStore(endpoint string) (*BlockStore, error) {
	var err error
	s := &BlockStore{
		RefCount: primitive.RefCount{},
		client: bare.New(endpoint, rpc.NewClientFunc(func(conn *grpc.ClientConn) interface{} {
			return segpb.NewSegmentServerClient(conn)
		})),
		tracer: tracing.NewTracer("internal.store.BlockStore", trace.SpanKindClient),
	}
	_, err = s.connectAppendStream(context.Background())
	if err != nil {
		// TODO: check error
		return nil, err
	}
	_, err = s.connectReadStream(context.Background())
	if err != nil {
		// TODO: check error
		return nil, err
	}
	return s, nil
}

type streamstate string

var (
	stateRunning streamstate = "running"
	stateCLosed  streamstate = "closed"
)

type appendStreamCache struct {
	stream    segpb.SegmentServer_AppendToBlockStreamClient
	callbacks sync.Map
	state     streamstate
	once      sync.Once
}

func (a *appendStreamCache) isRunning() bool {
	return a.state == stateRunning
}

func (a *appendStreamCache) isClosed() bool {
	return a.state == stateCLosed
}

func (a *appendStreamCache) release() {
	a.releaseStream()
	a.releaseCallbacks()
}

func (a *appendStreamCache) releaseStream() {
	a.once.Do(func() {
		a.stream.CloseSend()
		a.state = stateCLosed
	})
}

func (a *appendStreamCache) releaseCallbacks() {
	a.callbacks.Range(func(key, value interface{}) bool {
		if value != nil {
			value.(appendCallback)(&segpb.AppendToBlockStreamResponse{
				ResponseCode: errpb.ErrorCode_CLOSED,
				ResponseMsg:  "append stream closed",
				Offsets:      []int64{},
			})
		}
		return true
	})
}

type readStreamCache struct {
	stream    segpb.SegmentServer_ReadFromBlockStreamClient
	callbacks sync.Map
	state     streamstate
	once      sync.Once
}

func (r *readStreamCache) isRunning() bool {
	return r.state == stateRunning
}

func (r *readStreamCache) isClosed() bool {
	return r.state == stateCLosed
}

func (r *readStreamCache) release() {
	r.releaseStream()
	r.releaseCallbacks()
}

func (r *readStreamCache) releaseStream() {
	r.once.Do(func() {
		r.stream.CloseSend()
		r.state = stateCLosed
	})
}

func (r *readStreamCache) releaseCallbacks() {
	r.callbacks.Range(func(key, value interface{}) bool {
		if value != nil {
			value.(readCallback)(&segpb.ReadFromBlockStreamResponse{
				ResponseCode: errpb.ErrorCode_CLOSED,
				ResponseMsg:  "read stream closed",
				Events: &cepb.CloudEventBatch{
					Events: []*cepb.CloudEvent{},
				},
			})
		}
		return true
	})
}

type BlockStore struct {
	primitive.RefCount
	client   rpc.Client
	tracer   *tracing.Tracer
	append   *appendStreamCache
	read     *readStreamCache
	appendMu sync.RWMutex
	readMu   sync.RWMutex
}

type appendCallback func(*segpb.AppendToBlockStreamResponse)
type readCallback func(*segpb.ReadFromBlockStreamResponse)

func (s *BlockStore) runAppendStreamRecv(ctx context.Context, append *appendStreamCache) {
	for {
		res, err := append.stream.Recv()
		if err != nil {
			log.Error(ctx, "append stream recv failed", map[string]interface{}{
				log.KeyError: err,
			})
			// only release the remaining callbacks after the stream connection is closed
			if err == io.EOF || append.isClosed() {
				append.releaseCallbacks()
				return
			}
			break
		}
		c, _ := append.callbacks.LoadAndDelete(res.Id)
		if c != nil {
			c.(appendCallback)(res)
		}
	}
}

func (s *BlockStore) runReadStreamRecv(ctx context.Context, read *readStreamCache) {
	for {
		res, err := read.stream.Recv()
		if err != nil {
			log.Error(ctx, "read stream recv failed", map[string]interface{}{
				log.KeyError: err,
			})
			// only release the remaining callbacks after the stream connection is closed
			if err == io.EOF || read.isClosed() {
				read.releaseCallbacks()
				return
			}
			break
		}
		c, _ := read.callbacks.LoadAndDelete(res.Id)
		if c != nil {
			c.(readCallback)(res)
		}
	}
}

func (s *BlockStore) connectAppendStream(ctx context.Context) (*appendStreamCache, error) {
	s.appendMu.RLock()
	if s.append != nil && s.append.isRunning() {
		defer s.appendMu.RUnlock()
		return s.append, nil
	}
	s.appendMu.RUnlock()
	s.appendMu.Lock()
	defer s.appendMu.Unlock()
	if s.append != nil && s.append.isRunning() { //double check
		return s.append, nil
	}

	client, err := s.client.Get(ctx)
	if err != nil {
		return nil, err
	}

	stream, err := client.(segpb.SegmentServerClient).AppendToBlockStream(ctx)
	if err != nil {
		log.Warning(ctx, "get append stream failed", map[string]interface{}{
			log.KeyError: err,
		})
		return nil, err
	}

	cache := &appendStreamCache{
		stream:    stream,
		state:     stateRunning,
		callbacks: sync.Map{},
	}
	s.append = cache
	go s.runAppendStreamRecv(ctx, cache)
	return cache, nil
}

func (s *BlockStore) connectReadStream(ctx context.Context) (*readStreamCache, error) {
	s.readMu.RLock()
	if s.read != nil && s.read.isRunning() {
		defer s.readMu.RUnlock()
		return s.read, nil
	}
	s.readMu.RUnlock()
	s.readMu.Lock()
	defer s.readMu.Unlock()
	if s.read != nil && s.read.isRunning() { //double check
		return s.read, nil
	}

	client, err := s.client.Get(ctx)
	if err != nil {
		return nil, err
	}

	s.read = &readStreamCache{}
	stream, err := client.(segpb.SegmentServerClient).ReadFromBlockStream(ctx)
	if err != nil {
		log.Warning(ctx, "get read stream failed", map[string]interface{}{
			log.KeyError: err,
		})
		return nil, err
	}

	cache := &readStreamCache{
		stream:    stream,
		state:     stateRunning,
		callbacks: sync.Map{},
	}
	s.read = cache
	go s.runReadStreamRecv(ctx, cache)
	return cache, nil
}

func (s *BlockStore) Endpoint() string {
	return s.client.Endpoint()
}

func (s *BlockStore) Close() {
	if s.append != nil {
		s.append.release()
	}
	if s.read != nil {
		s.read.release()
	}
	s.client.Close()
}

func (s *BlockStore) Append(ctx context.Context, block uint64, event *ce.Event) (int64, error) {
	_ctx, span := s.tracer.Start(ctx, "Append")
	defer span.End()

	eventpb, err := codec.ToProto(event)
	if err != nil {
		return -1, err
	}
	req := &segpb.AppendToBlockRequest{
		BlockId: block,
		Events: &cepb.CloudEventBatch{
			Events: []*cepb.CloudEvent{eventpb},
		},
	}

	client, err := s.client.Get(_ctx)
	if err != nil {
		return -1, err
	}

	res, err := client.(segpb.SegmentServerClient).AppendToBlock(_ctx, req)
	if err != nil {
		return -1, err
	}
	return res.GetOffsets()[0], nil
}

func (s *BlockStore) AppendManyStream(ctx context.Context, block uint64, events []*ce.Event) ([]int64, error) {
	_ctx, span := s.tracer.Start(ctx, "AppendManyStream")
	defer span.End()

	var resp *segpb.AppendToBlockStreamResponse
	append, err := s.connectAppendStream(_ctx)
	if err != nil {
		return nil, err
	}

	// generate unique opaqueID
	opaqueID := rand.New(rand.NewSource(time.Now().UnixNano())).Uint64()

	//TODO(jiangkai): delete the reference of CloudEvents/v2 in Vanus
	eventpbs := make([]*cepb.CloudEvent, len(events))
	for idx := range events {
		eventpb, err := codec.ToProto(events[idx])
		if err != nil {
			return nil, err
		}
		eventpbs[idx] = eventpb
	}

	donec := make(chan struct{})
	append.callbacks.Store(opaqueID, appendCallback(func(res *segpb.AppendToBlockStreamResponse) {
		resp = res
		close(donec)
	}))

	req := &segpb.AppendToBlockStreamRequest{
		Id:      opaqueID,
		BlockId: block,
		Events: &cepb.CloudEventBatch{
			Events: eventpbs,
		},
	}

	if err = append.stream.Send(req); err != nil {
		log.Error(ctx, "append stream send failed", map[string]interface{}{
			log.KeyError: err,
		})
		// close the current stream connection
		append.releaseStream()
		// reset new stream connections
		s.connectAppendStream(ctx)
		c, _ := append.callbacks.LoadAndDelete(opaqueID)
		if c != nil {
			c.(appendCallback)(&segpb.AppendToBlockStreamResponse{
				Id:           opaqueID,
				ResponseCode: errpb.ErrorCode_CLOSED,
				ResponseMsg:  "append stream closed",
				Offsets:      []int64{},
			})
		}
		return nil, err
	}

	select {
	case <-donec:
	case <-_ctx.Done():
		c, _ := append.callbacks.LoadAndDelete(opaqueID)
		if c != nil {
			c.(appendCallback)(&segpb.AppendToBlockStreamResponse{
				Id:           opaqueID,
				ResponseCode: errpb.ErrorCode_CONTEXT_CANCELED,
				ResponseMsg:  "append stream context canceled",
				Offsets:      []int64{},
			})
		}
	}

	if resp.ResponseCode == errpb.ErrorCode_FULL {
		log.Warning(ctx, "block append failed cause the segment is full", nil)
		return nil, errors.ErrFull.WithMessage("segment is full")
	}

	if resp.ResponseCode != errpb.ErrorCode_SUCCESS {
		log.Warning(ctx, "block append failed cause unknown error", map[string]interface{}{
			"code":    resp.ResponseCode,
			"message": resp.ResponseMsg,
		})
		return nil, errors.ErrUnknown.WithMessage("append many stream failed")
	}

	return resp.Offsets, nil
}

func (s *BlockStore) Read(
	ctx context.Context, block uint64, offset int64, size int16, pollingTimeout uint32,
) ([]*ce.Event, error) {
	ctx, span := s.tracer.Start(ctx, "Read")
	defer span.End()

	req := &segpb.ReadFromBlockRequest{
		BlockId:                     block,
		Offset:                      offset,
		Number:                      int64(size),
		PollingTimeoutInMillisecond: pollingTimeout,
	}

	client, err := s.client.Get(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := client.(segpb.SegmentServerClient).ReadFromBlock(ctx, req)
	if err != nil {
		return nil, err
	}

	if batch := resp.GetEvents(); batch != nil {
		if eventpbs := batch.GetEvents(); len(eventpbs) > 0 {
			events := make([]*ce.Event, 0, len(eventpbs))
			for _, eventpb := range eventpbs {
				event, err2 := codec.FromProto(eventpb)
				if err2 != nil {
					// TODO: return events or error?
					return events, err2
				}
				events = append(events, event)
			}
			return events, nil
		}
	}

	return []*ce.Event{}, err
}

func (s *BlockStore) ReadStream(
	ctx context.Context, block uint64, offset int64, size int16, pollingTimeout uint32,
) ([]*ce.Event, error) {
	_ctx, span := s.tracer.Start(ctx, "ReadStream")
	defer span.End()

	var resp *segpb.ReadFromBlockStreamResponse
	read, err := s.connectReadStream(_ctx)
	if err != nil {
		return []*ce.Event{}, err
	}

	// generate unique RequestId
	opaqueID := rand.New(rand.NewSource(time.Now().UnixNano())).Uint64()

	donec := make(chan struct{})
	read.callbacks.Store(opaqueID, readCallback(func(res *segpb.ReadFromBlockStreamResponse) {
		resp = res
		close(donec)
	}))

	req := &segpb.ReadFromBlockStreamRequest{
		BlockId:                     block,
		Offset:                      offset,
		Number:                      int64(size),
		PollingTimeoutInMillisecond: pollingTimeout,
	}

	if err = s.read.stream.Send(req); err != nil {
		log.Error(ctx, "read stream send failed", map[string]interface{}{
			log.KeyError: err,
		})
		read.releaseStream()
		s.connectReadStream(ctx)
		c, _ := read.callbacks.LoadAndDelete(opaqueID)
		if c != nil {
			c.(readCallback)(&segpb.ReadFromBlockStreamResponse{
				Id:           opaqueID,
				ResponseCode: errpb.ErrorCode_CLOSED,
				ResponseMsg:  "read stream closed",
				Events: &cepb.CloudEventBatch{
					Events: []*cepb.CloudEvent{},
				},
			})
		}
		return []*ce.Event{}, err
	}

	select {
	case <-donec:
	case <-_ctx.Done():
		c, _ := read.callbacks.LoadAndDelete(opaqueID)
		if c != nil {
			c.(readCallback)(&segpb.ReadFromBlockStreamResponse{
				Id:           opaqueID,
				ResponseCode: errpb.ErrorCode_CONTEXT_CANCELED,
				ResponseMsg:  "read stream context canceled",
				Events: &cepb.CloudEventBatch{
					Events: []*cepb.CloudEvent{},
				},
			})
		}
	}

	if resp.ResponseCode != errpb.ErrorCode_SUCCESS {
		log.Warning(ctx, "block read failed cause unknown error", map[string]interface{}{
			"code":    resp.ResponseCode,
			"message": resp.ResponseMsg,
		})
		return []*ce.Event{}, errors.ErrUnknown.WithMessage("read stream failed")
	}

	if batch := resp.GetEvents(); batch != nil {
		if eventpbs := batch.GetEvents(); len(eventpbs) > 0 {
			events := make([]*ce.Event, 0, len(eventpbs))
			for _, eventpb := range eventpbs {
				event, err2 := codec.FromProto(eventpb)
				if err2 != nil {
					// TODO: return events or error?
					return events, err2
				}
				events = append(events, event)
			}
			return events, nil
		}
	}
	return []*ce.Event{}, errors.ErrUnknown
}

func (s *BlockStore) LookupOffset(ctx context.Context, blockID uint64, t time.Time) (int64, error) {
	ctx, span := s.tracer.Start(ctx, "LookupOffset")
	defer span.End()

	req := &segpb.LookupOffsetInBlockRequest{
		BlockId: blockID,
		Stime:   t.UnixMilli(),
	}

	client, err := s.client.Get(ctx)
	if err != nil {
		return -1, err
	}

	res, err := client.(segpb.SegmentServerClient).LookupOffsetInBlock(ctx, req)
	if err != nil {
		return -1, err
	}
	return res.Offset, nil
}
