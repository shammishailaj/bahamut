// Author: Antoine Mercadal
// See LICENSE file for full LICENSE
// Copyright 2016 Aporeto.

package bahamut

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aporeto-inc/elemental"
	"golang.org/x/net/websocket"

	opentracing "github.com/opentracing/opentracing-go"
)

type wsAPISession struct {
	processorFinder processorFinderFunc
	eventPusher     eventPusherFunc
	requests        chan *elemental.Request
	*wsSession
}

func newWSAPISession(ws *websocket.Conn, config Config, unregister unregisterFunc, processorFinder processorFinderFunc, eventPusher eventPusherFunc) internalWSSession {

	return &wsAPISession{
		wsSession:       newWSSession(ws, config, unregister, opentracing.StartSpan("bahamut.session.api")),
		processorFinder: processorFinder,
		eventPusher:     eventPusher,
		requests:        make(chan *elemental.Request, 8),
	}
}

func (s *wsAPISession) String() string {

	return fmt.Sprintf("<apisession id:%s parameters: %v>",
		s.id,
		s.parameters,
	)
}

func (s *wsAPISession) read() {

	for {
		request := elemental.NewRequestWithContext(s.context)
		request.ClientIP = s.remoteAddr

		if err := websocket.JSON.Receive(s.socket, request); err != nil {
			if _, ok := err.(*json.SyntaxError); !ok {
				s.cancel()
				s.stopAll <- true
				return
			}

			response := elemental.NewResponse()
			response.Request = request
			writeWebSocketError(s.socket, response, elemental.NewError("Bad Request", "Invalid JSON", "bahamut", http.StatusBadRequest))
		}

		select {
		case s.requests <- request:
		case <-s.stopRead:
			s.cancel()
			return
		}
	}
}

func (s *wsAPISession) listen() {

	go s.read()
	defer s.stop()

	for {
		select {
		case request := <-s.requests:

			// We backport the token of the session into the request if we don't have an explicit one given in the request.
			if request.Password == "" {
				if t := s.GetToken(); t != "" {
					request.Username = "Bearer"
					request.Password = t
				}
			}

			// And we set the TLSConnectionState
			request.TLSConnectionState = s.TLSConnectionState()

			switch request.Operation {

			case elemental.OperationRetrieveMany:
				go s.handleRetrieveMany(request)

			case elemental.OperationRetrieve:
				go s.handleRetrieve(request)

			case elemental.OperationCreate:
				go s.handleCreate(request)

			case elemental.OperationUpdate:
				go s.handleUpdate(request)

			case elemental.OperationDelete:
				go s.handleDelete(request)

			case elemental.OperationInfo:
				go s.handleInfo(request)

			case elemental.OperationPatch:
				go s.handlePatch(request)
			}

		case <-s.stopAll:
			s.cancel()
			return
		}
	}
}

// while this function is the same for wsAPISession and wsPushSession
// it has to be written in both of the struc instead of wsSession as
// if would call s.unregister using *wsSession and not a *wsAPISession
func (s *wsAPISession) stop() {

	s.stopRead <- true
	s.stopWrite <- true

	s.unregister(s)
	s.socket.Close() // nolint: errcheck
}

func (s *wsAPISession) handleEventualPanic(response *elemental.Response) {

	err := handleRecoveredPanic(recover(), response.Request)
	if err == nil {
		return
	}

	writeWebSocketError(s.socket, response, err)
}

func (s *wsAPISession) handleRetrieveMany(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	request.StartTracing()
	defer request.FinishTracing()
	defer s.handleEventualPanic(response)

	parentIdentity := request.ParentIdentity
	if parentIdentity.IsEmpty() {
		parentIdentity = elemental.RootIdentity
	}

	if !elemental.IsRetrieveManyAllowed(s.config.Model.RelationshipsRegistry[request.Version], request.Identity, parentIdentity) {
		writeWebSocketError(s.socket, response, elemental.NewError("Not allowed", "RetrieveMany operation not allowed on "+request.Identity.Category, "bahamut", http.StatusMethodNotAllowed))
		return
	}

	ctx := NewContextWithRequest(request)

	runWSDispatcher(
		ctx,
		s.socket,
		response,
		func() error {
			return dispatchRetrieveManyOperation(
				ctx,
				s.processorFinder,
				s.config.Model.IdentifiablesFactory,
				s.config.Security.RequestAuthenticators,
				s.config.Security.Authorizers,
				s.eventPusher,
				s.config.Security.Auditer,
			)
		},
	)
}

func (s *wsAPISession) handleRetrieve(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	request.StartTracing()
	defer request.FinishTracing()
	defer s.handleEventualPanic(response)

	if !elemental.IsRetrieveAllowed(s.config.Model.RelationshipsRegistry[request.Version], request.Identity) || !request.ParentIdentity.IsEmpty() {
		writeWebSocketError(s.socket, response, elemental.NewError("Not allowed", "Retrieve operation not allowed on "+request.Identity.Name, "bahamut", http.StatusMethodNotAllowed))
		return
	}

	ctx := NewContextWithRequest(request)

	runWSDispatcher(
		ctx,
		s.socket,
		response,
		func() error {
			return dispatchRetrieveOperation(
				ctx,
				s.processorFinder,
				s.config.Model.IdentifiablesFactory,
				s.config.Security.RequestAuthenticators,
				s.config.Security.Authorizers,
				s.eventPusher,
				s.config.Security.Auditer,
			)
		},
	)
}

func (s *wsAPISession) handleCreate(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	request.StartTracing()
	defer request.FinishTracing()
	defer s.handleEventualPanic(response)

	parentIdentity := request.ParentIdentity
	if parentIdentity.IsEmpty() {
		parentIdentity = elemental.RootIdentity
	}

	if !elemental.IsCreateAllowed(s.config.Model.RelationshipsRegistry[request.Version], request.Identity, parentIdentity) {
		writeWebSocketError(s.socket, response, elemental.NewError("Not allowed", "Create operation not allowed on "+request.Identity.Name, "bahamut", http.StatusMethodNotAllowed))
		return
	}

	ctx := NewContextWithRequest(request)

	runWSDispatcher(
		ctx,
		s.socket,
		response,
		func() error {
			return dispatchCreateOperation(
				ctx,
				s.processorFinder,
				s.config.Model.IdentifiablesFactory,
				s.config.Security.RequestAuthenticators,
				s.config.Security.Authorizers,
				s.eventPusher,
				s.config.Security.Auditer,
				s.config.Model.ReadOnly,
				s.config.Model.ReadOnlyExcludedIdentities,
			)
		},
	)
}

func (s *wsAPISession) handleUpdate(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	request.StartTracing()
	defer request.FinishTracing()
	defer s.handleEventualPanic(response)

	if !elemental.IsUpdateAllowed(s.config.Model.RelationshipsRegistry[request.Version], request.Identity) || !request.ParentIdentity.IsEmpty() {
		writeWebSocketError(s.socket, response, elemental.NewError("Not allowed", "Update operation not allowed on "+request.Identity.Name, "bahamut", http.StatusMethodNotAllowed))
		return
	}

	ctx := NewContextWithRequest(request)

	runWSDispatcher(
		ctx,
		s.socket,
		response,
		func() error {
			return dispatchUpdateOperation(
				ctx,
				s.processorFinder,
				s.config.Model.IdentifiablesFactory,
				s.config.Security.RequestAuthenticators,
				s.config.Security.Authorizers,
				s.eventPusher,
				s.config.Security.Auditer,
				s.config.Model.ReadOnly,
				s.config.Model.ReadOnlyExcludedIdentities,
			)
		},
	)
}

func (s *wsAPISession) handleDelete(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	request.StartTracing()
	defer request.FinishTracing()
	defer s.handleEventualPanic(response)

	if !elemental.IsDeleteAllowed(s.config.Model.RelationshipsRegistry[request.Version], request.Identity) || !request.ParentIdentity.IsEmpty() {
		writeWebSocketError(s.socket, response, elemental.NewError("Not allowed", "Delete operation not allowed on "+request.Identity.Name, "bahamut", http.StatusMethodNotAllowed))
		return
	}

	ctx := NewContextWithRequest(request)

	runWSDispatcher(
		ctx,
		s.socket,
		response,
		func() error {
			return dispatchDeleteOperation(
				ctx,
				s.processorFinder,
				s.config.Model.IdentifiablesFactory,
				s.config.Security.RequestAuthenticators,
				s.config.Security.Authorizers,
				s.eventPusher,
				s.config.Security.Auditer,
				s.config.Model.ReadOnly,
				s.config.Model.ReadOnlyExcludedIdentities,
			)
		},
	)
}

func (s *wsAPISession) handleInfo(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	request.StartTracing()
	defer request.FinishTracing()
	defer s.handleEventualPanic(response)

	parentIdentity := request.ParentIdentity
	if parentIdentity.IsEmpty() {
		parentIdentity = elemental.RootIdentity
	}

	if !elemental.IsInfoAllowed(s.config.Model.RelationshipsRegistry[request.Version], request.Identity, parentIdentity) {
		writeWebSocketError(s.socket, response, elemental.NewError("Not allowed", "Info operation not allowed on "+request.Identity.Category, "bahamut", http.StatusMethodNotAllowed))
		return
	}

	ctx := NewContextWithRequest(request)
	runWSDispatcher(
		ctx,
		s.socket,
		response,
		func() error {
			return dispatchInfoOperation(
				ctx,
				s.processorFinder,
				s.config.Model.IdentifiablesFactory,
				s.config.Security.RequestAuthenticators,
				s.config.Security.Authorizers,
				s.config.Security.Auditer,
			)
		},
	)
}

func (s *wsAPISession) handlePatch(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	request.StartTracing()
	defer request.FinishTracing()
	defer s.handleEventualPanic(response)

	parentIdentity := request.ParentIdentity
	if parentIdentity.IsEmpty() {
		parentIdentity = elemental.RootIdentity
	}

	if !elemental.IsPatchAllowed(s.config.Model.RelationshipsRegistry[request.Version], request.Identity, parentIdentity) {
		writeWebSocketError(s.socket, response, elemental.NewError("Not allowed", "Patch operation not allowed on "+request.Identity.Name, "bahamut", http.StatusMethodNotAllowed))
		return
	}

	ctx := NewContextWithRequest(request)
	runWSDispatcher(
		ctx,
		s.socket,
		response,
		func() error {
			return dispatchPatchOperation(
				ctx,
				s.processorFinder,
				s.config.Model.IdentifiablesFactory,
				s.config.Security.RequestAuthenticators,
				s.config.Security.Authorizers,
				s.eventPusher,
				s.config.Security.Auditer,
				s.config.Model.ReadOnly,
				s.config.Model.ReadOnlyExcludedIdentities,
			)
		},
	)
}
