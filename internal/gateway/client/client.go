package client

import (
	"context"
	"errors"
	v2 "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	eb "github.com/linkall-labs/vanus/client"
	"github.com/linkall-labs/vanus/client/pkg/api"
	"github.com/linkall-labs/vanus/internal/primitive"
	"github.com/linkall-labs/vanus/observability/log"
	"sync"
)

type Client struct {
	busWriter sync.Map
	client    eb.Client
}

func NewClient(addrs ...string) *Client {
	return &Client{
		client: eb.Connect(addrs),
	}
}

func (ga *Client) Send(ctx context.Context, event *v2.Event) (string, error) {
	extensions := event.Extensions()
	ebName := extensions[primitive.XVanusEventbus].(string)
	if eventTime, ok := extensions[primitive.XVanusDeliveryTime]; ok {
		// validate event time
		if _, err := types.ParseTime(eventTime.(string)); err != nil {
			log.Error(ctx, "invalid format of event time", map[string]interface{}{
				log.KeyError: err,
				"eventTime":  eventTime.(string),
			})
			return "", errors.New("invalid delivery time")
		}
		ebName = primitive.TimerEventbusName
	}

	// TODO remove
	v, exist := ga.busWriter.Load(ebName)
	if !exist {
		v, _ = ga.busWriter.LoadOrStore(ebName, ga.client.Eventbus(ctx, ebName).Writer())
	}
	writer, _ := v.(api.BusWriter)
	eventID, err := writer.AppendOne(ctx, event)
	if err != nil {
		log.Warning(ctx, "append to failed", map[string]interface{}{
			log.KeyError: err,
			"eventbus":   ebName,
		})
		return "", err
	}
	return eventID, nil
}
