// Copyright 2022 Linkall Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file exceptreq compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed toreq writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/Jeffail/tunny"
	format "github.com/cloudevents/sdk-go/binding/format/protobuf/v2"
	"github.com/linkall-labs/vanus/internal/gateway/client"
	"net"
	"net/http"
	"runtime/debug"
	"sync"

	gpb "github.com/cloudevents/sdk-go/protocol/grpc/v2/pb"
	recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	eb "github.com/linkall-labs/vanus/client"
	"github.com/linkall-labs/vanus/client/pkg/api"
	"github.com/linkall-labs/vanus/client/pkg/option"
	"github.com/linkall-labs/vanus/client/pkg/policy"
	"github.com/linkall-labs/vanus/internal/primitive/interceptor/errinterceptor"
	"github.com/linkall-labs/vanus/internal/primitive/vanus"
	"github.com/linkall-labs/vanus/observability/log"
	"github.com/linkall-labs/vanus/observability/tracing"
	"github.com/linkall-labs/vanus/pkg/controller"
	ctrlpb "github.com/linkall-labs/vanus/proto/pkg/controller"
	proxypb "github.com/linkall-labs/vanus/proto/pkg/proxy"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	maximumNumberPerGetRequest = 64
)

var (
	errInvalidEventbus = errors.New("the eventbus name can't be empty")
)

type Config struct {
	Endpoints              []string
	ProxyPort              int
	CloudEventReceiverPort int
	Credentials            credentials.TransportCredentials
	GRPCReflectionEnable   bool
}

var (
	_ proxypb.ControllerProxyServer = &ControllerProxy{}
	_ gpb.CloudEventsServiceServer  = &ControllerProxy{}
)

type ControllerProxy struct {
	cfg          Config
	tracer       *tracing.Tracer
	client       eb.Client
	eventbusCtrl ctrlpb.EventBusControllerClient
	eventlogCtrl ctrlpb.EventLogControllerClient
	triggerCtrl  ctrlpb.TriggerControllerClient
	grpcSrv      *grpc.Server
	cli          *client.Client
	ch           chan struct{}
	s            gpb.CloudEventsService_SendServer
	pool         *tunny.Pool
}

func NewControllerProxy(cfg Config, cli *client.Client) *ControllerProxy {
	return &ControllerProxy{
		cfg:          cfg,
		cli:          cli,
		client:       eb.Connect(cfg.Endpoints),
		tracer:       tracing.NewTracer("controller-proxy", trace.SpanKindServer),
		eventbusCtrl: controller.NewEventbusClient(cfg.Endpoints, cfg.Credentials),
		eventlogCtrl: controller.NewEventlogClient(cfg.Endpoints, cfg.Credentials),
		triggerCtrl:  controller.NewTriggerClient(cfg.Endpoints, cfg.Credentials),
		ch:           make(chan struct{}, 2048),
	}
}

func (cp *ControllerProxy) Start() error {
	recoveryOpt := recovery.WithRecoveryHandlerContext(
		func(ctx context.Context, p interface{}) error {
			log.Error(ctx, "goroutine panicked", map[string]interface{}{
				log.KeyError: fmt.Sprintf("%v", p),
				"stack":      string(debug.Stack()),
			})
			return status.Errorf(codes.Internal, "%v", p)
		},
	)

	cp.grpcSrv = grpc.NewServer(
		grpc.ChainStreamInterceptor(
			errinterceptor.StreamServerInterceptor(),
			recovery.StreamServerInterceptor(recoveryOpt),
			otelgrpc.StreamServerInterceptor(),
		),
		grpc.ChainUnaryInterceptor(
			errinterceptor.UnaryServerInterceptor(),
			recovery.UnaryServerInterceptor(recoveryOpt),
			otelgrpc.UnaryServerInterceptor(),
		),
	)

	// for debug in developing stage
	if cp.cfg.GRPCReflectionEnable {
		reflection.Register(cp.grpcSrv)
	}

	proxypb.RegisterControllerProxyServer(cp.grpcSrv, cp)
	gpb.RegisterCloudEventsServiceServer(cp.grpcSrv, cp)
	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", cp.cfg.ProxyPort))
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = cp.grpcSrv.Serve(listen)
		if err != nil {
			panic(fmt.Sprintf("start grpc proxy failed: %s", err.Error()))
		}
		wg.Done()
	}()
	cp.pool = tunny.NewFunc(16, func(payload interface{}) interface{} {
		var result []byte

		// TODO: Something CPU heavy with payload

		return result
	})
	go cp.receive()
	log.Info(context.Background(), "the grpc proxy ready to work", nil)
	return nil
}

func (cp *ControllerProxy) Stop() {
	if cp.grpcSrv != nil {
		cp.grpcSrv.GracefulStop()
	}
}

func (cp *ControllerProxy) ClusterInfo(_ context.Context, _ *emptypb.Empty) (*proxypb.ClusterInfoResponse, error) {
	return &proxypb.ClusterInfoResponse{
		CloudeventsPort: int64(cp.cfg.CloudEventReceiverPort),
		ProxyPort:       int64(cp.cfg.ProxyPort),
	}, nil
}

func (cp *ControllerProxy) LookupOffset(ctx context.Context,
	req *proxypb.LookupOffsetRequest) (*proxypb.LookupOffsetResponse, error) {
	elList := make([]api.Eventlog, 0)
	if req.EventlogId > 0 {
		id := vanus.NewIDFromUint64(req.EventlogId)
		l, err := cp.client.Eventbus(ctx, req.GetEventbus()).GetLog(ctx, id.Uint64())
		if err != nil {
			return nil, err
		}
		elList = append(elList, l)
	} else {
		ls, err := cp.client.Eventbus(ctx, req.GetEventbus()).ListLog(ctx)
		if err != nil {
			return nil, err
		}
		elList = ls
	}
	if len(elList) == 0 {
		return nil, errors.New("eventbus not found")
	}
	res := &proxypb.LookupOffsetResponse{
		Offsets: map[uint64]int64{},
	}
	for idx := range elList {
		l := elList[idx]
		off, err := l.QueryOffsetByTime(ctx, req.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup offset: %w", err)
		}
		res.Offsets[l.ID()] = off
	}
	return res, nil
}

func (cp *ControllerProxy) GetEvent(ctx context.Context,
	req *proxypb.GetEventRequest) (*proxypb.GetEventResponse, error) {
	if req.GetEventbus() == "" {
		return nil, errInvalidEventbus
	}

	if req.EventId != "" {
		return cp.getByEventID(ctx, req)
	}

	var (
		offset = req.Offset
		num    = req.Number
	)

	if offset < 0 {
		offset = 0
	}

	if num > maximumNumberPerGetRequest {
		num = maximumNumberPerGetRequest
	}

	ls, err := cp.client.Eventbus(ctx, req.GetEventbus()).ListLog(ctx)
	if err != nil {
		return nil, err
	}

	events, _, _, err := cp.client.Eventbus(ctx, req.GetEventbus()).Reader(
		option.WithDisablePolling(),
		option.WithReadPolicy(policy.NewManuallyReadPolicy(ls[0], offset)),
		option.WithBatchSize(int(num)),
	).Read(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]*wrapperspb.BytesValue, len(events))
	for idx, v := range events {
		data, _ := v.MarshalJSON()
		results[idx] = wrapperspb.Bytes(data)
	}
	return &proxypb.GetEventResponse{
		Events: results,
	}, nil
}

// getByEventID why added this? can it be deleted?
func (cp *ControllerProxy) getByEventID(ctx context.Context,
	req *proxypb.GetEventRequest) (*proxypb.GetEventResponse, error) {
	logID, off, err := decodeEventID(req.EventId)
	if err != nil {
		return nil, err
	}

	l, err := cp.client.Eventbus(ctx, req.GetEventbus()).GetLog(ctx, logID)
	if err != nil {
		return nil, err
	}

	events, _, _, err := cp.client.Eventbus(ctx, req.GetEventbus()).Reader(
		option.WithReadPolicy(policy.NewManuallyReadPolicy(l, off)),
		option.WithDisablePolling(),
	).Read(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]*wrapperspb.BytesValue, len(events))
	for idx, v := range events {
		data, _ := v.MarshalJSON()
		results[idx] = wrapperspb.Bytes(data)
	}
	return &proxypb.GetEventResponse{
		Events: results,
	}, nil
}

func decodeEventID(eventID string) (uint64, int64, error) {
	decoded, err := base64.StdEncoding.DecodeString(eventID)
	if err != nil {
		return 0, 0, err
	}
	if len(decoded) != 16 { // fixed length
		return 0, 0, fmt.Errorf("invalid event id")
	}
	logID := binary.BigEndian.Uint64(decoded[0:8])
	off := binary.BigEndian.Uint64(decoded[8:16])
	return logID, int64(off), nil
}

func (cp *ControllerProxy) Send(s gpb.CloudEventsService_SendServer) error {
	cp.s = s

	for {
		r, err := s.Recv()
		if err != nil {
			log.Info(nil, err.Error(), nil)
			break
		}

		cp.ch <- struct{}{}
		go func(req *gpb.CloudEventBatch) {
			e, err := format.FromProto(req.Events[0])
			if err != nil {
				_ = cp.s.Send(&gpb.SendResponse{
					Status:    http.StatusInternalServerError,
					Message:   err.Error(),
					RequestId: req.RequestId,
				})
				return
			}
			_, err = cp.cli.Send(context.Background(), e)
			if err != nil {
				_ = cp.s.Send(&gpb.SendResponse{
					Status:    http.StatusInternalServerError,
					Message:   err.Error(),
					RequestId: req.RequestId,
				})
				return
			}
			_ = cp.s.Send(&gpb.SendResponse{
				Status:    http.StatusOK,
				RequestId: req.RequestId,
			})
			<-cp.ch
		}(r)
	}
	return nil
}

func (cp *ControllerProxy) receive() {
	//for {
	//	for r := range cp.ch {
	//
	//	}
	//}
}

func (cp *ControllerProxy) process(in interface{}) interface{} {
	return nil
}
