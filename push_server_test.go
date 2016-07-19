// Author: Antoine Mercadal
// See LICENSE file for full LICENSE
// Copyright 2016 Aporeto.

package bahamut

import (
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"github.com/Shopify/sarama"
	"github.com/aporeto-inc/elemental"
	"github.com/go-zoo/bone"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPushServer_newPushServer(t *testing.T) {

	Convey("Given I create a new PushServer", t, func() {

		srv := newPushServer(PushServerConfig{}, bone.New())

		Convey("Then sessions should be initialized", func() {
			So(len(srv.sessions), ShouldEqual, 0)
		})

		Convey("Then register channel should be initialized", func() {
			var m chan *PushSession
			So(srv.register, ShouldHaveSameTypeAs, m)
		})

		Convey("Then unregister channel should be initialized", func() {
			var m chan *PushSession
			So(srv.unregister, ShouldHaveSameTypeAs, m)
		})

		Convey("Then stop channel should be initialized", func() {
			var m chan bool
			So(srv.close, ShouldHaveSameTypeAs, m)
		})

		Convey("Then events channel should be initialized", func() {
			var m chan *elemental.Event
			So(srv.events, ShouldHaveSameTypeAs, m)
		})
	})
}

func TestSession_registerSession(t *testing.T) {

	ts := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		var d []byte
		websocket.Message.Receive(ws, &d)
		websocket.Message.Send(ws, d)
	}))
	defer ts.Close()

	Convey("Given I have an PushServer and no registered session", t, func() {

		ws, _ := websocket.Dial("ws"+ts.URL[4:], "", ts.URL)
		defer ws.Close()

		handler := &testSessionHandler{}
		srv := newPushServer(MakePushServerConfig([]string{}, "", handler), bone.New())
		session := newPushSession(ws, srv)

		go srv.start()
		defer srv.stop()

		Convey("When I register a session", func() {

			srv.registerSession(session)
			srv.registerSession(session)

			Convey("Then there should be 1 registered session", func() {
				So(len(srv.sessions), ShouldEqual, 1)
			})

			Convey("Then my session handler should have one registered session", func() {
				So(handler.sessionCount, ShouldEqual, 1)
			})
		})
	})

	Convey("Given I have an PushServer and a registered session", t, func() {

		ws, _ := websocket.Dial("ws"+ts.URL[4:], "", ts.URL)
		defer ws.Close()

		handler := &testSessionHandler{}
		srv := newPushServer(MakePushServerConfig([]string{}, "", handler), bone.New())
		session := newPushSession(ws, srv)

		go srv.start()
		defer srv.stop()

		srv.registerSession(session)

		Convey("When I unregister a registered session", func() {

			srv.unregisterSession(session)
			srv.unregisterSession(session)

			Convey("Then there should be 0 registered session", func() {
				So(len(srv.sessions), ShouldEqual, 0)
			})

			Convey("Then my session handler should have zero registered session", func() {
				So(handler.sessionCount, ShouldEqual, 0)
			})
		})
	})
}

func TestSession_startStop(t *testing.T) {

	ts := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {}))
	defer ts.Close()

	Convey("Given I have a started PushServer with a session", t, func() {

		ws, _ := websocket.Dial("ws"+ts.URL[4:], "", ts.URL)
		defer ws.Close()

		srv := newPushServer(PushServerConfig{}, bone.New())
		session := newPushSession(ws, srv)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			srv.start()
			wg.Done()
		}()

		srv.registerSession(session)

		Convey("When I stop it", func() {

			srv.stop()
			wg.Wait()

			Convey("Then the len of sessions should be 0", func() {
				So(len(srv.sessions), ShouldEqual, 0)
			})
		})
	})
}

func TestSession_HandleConnection(t *testing.T) {

	ts := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		var d []byte
		websocket.Message.Receive(ws, &d)
		websocket.Message.Send(ws, d)
	}))
	defer ts.Close()

	Convey("Given I create a new PushServer", t, func() {

		srv := newPushServer(PushServerConfig{}, bone.New())
		ws, _ := websocket.Dial("ws"+ts.URL[4:], "", ts.URL)
		defer ws.Close()

		Convey("When call handleConnection", func() {

			go srv.handleConnection(ws)

			var registered bool
			select {
			case <-srv.register:
				registered = true
				break
			case <-time.After(300 * time.Millisecond):
				break
			}

			Convey("Then a new session should be registered", func() {
				So(registered, ShouldBeTrue)
			})
		})
	})
}

func TestSession_PushEvents(t *testing.T) {

	Convey("Given I create a new PushServer", t, func() {

		srv := newPushServer(PushServerConfig{}, bone.New())

		Convey("When I push an event", func() {

			inEvent := elemental.NewEvent(elemental.EventCreate, NewList())
			srv.pushEvents(inEvent)

			var outEvent *elemental.Event
			select {
			case outEvent = <-srv.events:
				break
			case <-time.After(300 * time.Millisecond):
				break
			}

			Convey("Then the event should be sent throught the event channel", func() {
				So(outEvent, ShouldEqual, inEvent)
			})
		})
	})
}

func TestSession_GlobalEvents(t *testing.T) {

	Convey("Given I have a started PushServer a session", t, func() {

		broker := sarama.NewMockBroker(t, 1)
		metadataResponse := new(sarama.MetadataResponse)
		metadataResponse.AddBroker(broker.Addr(), broker.BrokerID())
		metadataResponse.AddTopicPartition("topic", 0, broker.BrokerID(), nil, nil, sarama.ErrNoError)
		broker.Returns(metadataResponse)
		defer broker.Close()

		config := MakePushServerConfig([]string{broker.Addr()}, "topic", nil)
		srv := newPushServer(config, bone.New())

		go srv.start()

		Convey("When push an event", func() {

			srv.pushEvents(elemental.NewEvent(elemental.EventCreate, NewList()))

			time.Sleep(5 * time.Millisecond)

			Convey("Then kafka should have received the message", func() {
				So(len(broker.History()), ShouldEqual, 2)
			})
		})
	})
}

func TestSession_LocalEvents(t *testing.T) {

	ts := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		var d []byte
		websocket.Message.Receive(ws, &d)
		websocket.Message.Send(ws, d)
	}))
	defer ts.Close()

	Convey("Given I have a started PushServer a session", t, func() {

		ws1, _ := websocket.Dial("ws"+ts.URL[4:], "", ts.URL)
		defer ws1.Close()

		srv := newPushServer(PushServerConfig{}, bone.New())
		session1 := newPushSession(ws1, srv)

		go srv.start()
		srv.registerSession(session1)

		Convey("When push an event", func() {

			srv.pushEvents(elemental.NewEvent(elemental.EventCreate, NewList()))

			var evt string
			select {
			case evt = <-session1.events:
				break
			case <-time.After(3 * time.Millisecond):
				break
			}

			Convey("Then output event should be correct", func() {
				So(evt, ShouldNotBeEmpty)
			})
		})

		Convey("When push an event with an UnmarshalableList", func() {

			srv.pushEvents(elemental.NewEvent(elemental.EventCreate, NewUnmarshalableList()))

			var evt string
			select {
			case evt = <-session1.events:
				break
			case <-time.After(3 * time.Millisecond):
				break
			}

			Convey("Then output event should be correct", func() {
				So(evt, ShouldBeEmpty)
			})
		})
	})
}
