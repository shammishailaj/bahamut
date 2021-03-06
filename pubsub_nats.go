// Copyright 2019 Aporeto Inc.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bahamut

import (
	"crypto/tls"
	"fmt"
	"time"

	"go.aporeto.io/elemental"

	"github.com/gofrs/uuid"
	nats "github.com/nats-io/go-nats"
	"go.uber.org/zap"
)

type natsPubSub struct {
	natsURL        string
	client         *nats.Conn
	retryInterval  time.Duration
	publishTimeout time.Duration
	retryNumber    int
	clientID       string
	clusterID      string
	password       string
	username       string
	tlsConfig      *tls.Config
}

// NewNATSPubSubClient returns a new PubSubClient backend by Nats.
func NewNATSPubSubClient(natsURL string, options ...NATSOption) PubSubClient {

	n := &natsPubSub{
		natsURL:        natsURL,
		retryInterval:  5 * time.Second,
		publishTimeout: 8 * time.Second,
		retryNumber:    5,
		clientID:       uuid.Must(uuid.NewV4()).String(),
		clusterID:      "test-cluster",
	}

	for _, opt := range options {
		opt(n)
	}

	return n
}

func (p *natsPubSub) Publish(publication *Publication, opts ...PubSubOptPublish) error {

	config := natsPublishConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	if p.client == nil {
		return fmt.Errorf("not connected to nats. messages dropped")
	}

	data, err := elemental.Encode(elemental.EncodingTypeMSGPACK, publication)
	if err != nil {
		return fmt.Errorf("unable to encode publication. message dropped: %s", err)
	}

	if config.replyValidator == nil {
		return p.client.Publish(publication.Topic, data)
	}

	msg, err := p.client.RequestWithContext(config.ctx, publication.Topic, data)
	if err != nil {
		return err
	}

	return config.replyValidator(msg)
}

func (p *natsPubSub) Subscribe(pubs chan *Publication, errors chan error, topic string, opts ...PubSubOptSubscribe) func() {

	config := natsSubscribeConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	var sub *nats.Subscription
	var err error

	handler := func(m *nats.Msg) {
		publication := NewPublication(topic)

		if e := elemental.Decode(elemental.EncodingTypeMSGPACK, m.Data, publication); e != nil {
			zap.L().Error("Unable to decode publication envelope. Message dropped.", zap.Error(e))
			return
		}

		if m.Reply != "" {

			resp := ackMessage
			if config.replier != nil {
				resp = config.replier(m)
			}

			if err := p.client.Publish(m.Reply, resp); err != nil {
				zap.L().Error("Unable to send requested reply", zap.Error(err))
				return
			}
		}

		pubs <- publication
	}

	if config.queueGroup == "" {
		sub, err = p.client.Subscribe(topic, handler)
	} else {
		sub, err = p.client.QueueSubscribe(topic, config.queueGroup, handler)
	}

	if err != nil {
		errors <- err
		return func() {}
	}

	return func() { _ = sub.Unsubscribe() }
}

func (p *natsPubSub) Connect() Waiter {

	abort := make(chan struct{})
	connected := make(chan bool)

	go func() {

		// First, we create a connection to the nats cluster.
		for p.client == nil {

			var err error

			if p.username != "" || p.password != "" {
				p.client, err = nats.Connect(p.natsURL, nats.UserInfo(p.username, p.password), nats.Secure(p.tlsConfig))
			} else {
				p.client, err = nats.Connect(p.natsURL, nats.Secure(p.tlsConfig))
			}

			if err == nil {
				break
			}

			zap.L().Warn("Unable to connect to nats cluster. Retrying",
				zap.String("url", p.natsURL),
				zap.Duration("retry", p.retryInterval),
				zap.Error(err),
			)

			select {
			case <-time.After(p.retryInterval):
			case <-abort:
				connected <- false
				return
			}
		}
		connected <- true
	}()

	return connectionWaiter{
		ok:    connected,
		abort: abort,
	}
}

func (p *natsPubSub) Disconnect() error {

	if err := p.client.Flush(); err != nil {
		return err
	}

	p.client.Close()

	return nil
}

func (p *natsPubSub) Ping(timeout time.Duration) error {

	errChannel := make(chan error)

	go func() {
		if p.client.IsConnected() {
			errChannel <- nil
		} else if p.client.IsReconnecting() {
			errChannel <- fmt.Errorf("reconnecting")
		} else {
			errChannel <- fmt.Errorf("connection closed")
		}
	}()

	select {
	case <-time.After(timeout):
		return fmt.Errorf("connection timeout")
	case err := <-errChannel:
		return err
	}
}
