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
	"context"
	"fmt"
	"net/http"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"go.aporeto.io/elemental"
)

type handlerFunc func(*bcontext, config, processorFinderFunc, eventPusherFunc) *elemental.Response

func makeResponse(ctx *bcontext, response *elemental.Response, cleaner TraceCleaner) *elemental.Response {

	if ctx.redirect != "" {
		response.Redirect = ctx.redirect
		return response
	}

	var fields []log.Field
	defer func() {
		if span := opentracing.SpanFromContext(ctx.ctx); span != nil {
			span.LogFields(fields...)
			span.SetTag("status.code", response.StatusCode)
		}
	}()

	response.StatusCode = ctx.statusCode
	if response.StatusCode == 0 {
		switch ctx.request.Operation {
		case elemental.OperationCreate:
			response.StatusCode = http.StatusCreated
		case elemental.OperationInfo:
			response.StatusCode = http.StatusNoContent
		default:
			response.StatusCode = http.StatusOK
		}
	}

	if ctx.request.Operation == elemental.OperationRetrieveMany || ctx.request.Operation == elemental.OperationInfo {
		response.Total = ctx.count
		fields = append(fields, (log.Int("count-total", ctx.count)))
	}

	if msgs := ctx.messages; len(msgs) > 0 {
		response.Messages = msgs
		fields = append(fields, (log.Object("messages", msgs)))
	}

	if ctx.outputData == nil {
		response.StatusCode = http.StatusNoContent
		return response
	}

	var requestedFields []string
	if ctx.Request().Headers != nil {
		requestedFields = ctx.Request().Headers["X-Fields"]
	}

	elemental.ResetSecretAttributesValues(ctx.outputData)

	if len(requestedFields) > 0 {

		switch ident := ctx.outputData.(type) {
		case elemental.PlainIdentifiable:
			ctx.outputData = ident.ToSparse(requestedFields...)
		case elemental.PlainIdentifiables:
			ctx.outputData = ident.ToSparse(requestedFields...)
		}
	}

	if err := response.Encode(ctx.OutputData()); err != nil {
		panic(fmt.Errorf("unable to encode output data: %s", err))
	}

	data := response.Data[:]
	if cleaner != nil {
		data = cleaner(response.Request.Identity, data)
	}

	fields = append(fields, log.Object("response", string(data)))

	return response
}

func makeErrorResponse(ctx context.Context, response *elemental.Response, err error) *elemental.Response {

	if err == context.Canceled {
		return nil
	}

	outError := processError(ctx, err)
	response.StatusCode = outError.Code()

	if err := response.Encode(outError); err != nil {
		panic(fmt.Errorf("unable to encode error: %s", err))
	}

	return response
}

func handleEventualPanic(ctx context.Context, c chan error, disablePanicRecovery bool) {

	if err := handleRecoveredPanic(ctx, recover(), disablePanicRecovery); err != nil {
		c <- err
	}
}

func runDispatcher(ctx *bcontext, r *elemental.Response, d func() error, disablePanicRecovery bool, traceCleaner TraceCleaner) *elemental.Response {

	e := make(chan error)

	go func() {
		defer handleEventualPanic(ctx.ctx, e, disablePanicRecovery)
		select {
		case e <- d():
		default:
		}
	}()

	select {

	case <-ctx.ctx.Done():
		return makeErrorResponse(ctx.ctx, r, ctx.ctx.Err())

	case err := <-e:
		if err != nil {
			return makeErrorResponse(ctx.ctx, r, err)
		}

		return makeResponse(ctx, r, traceCleaner)
	}
}

func handleRetrieveMany(ctx *bcontext, cfg config, processorFinder processorFinderFunc, pusherFunc eventPusherFunc) (response *elemental.Response) {

	response = elemental.NewResponse(ctx.request)

	if !elemental.IsOperationAllowed(
		cfg.model.modelManagers[ctx.request.Version].Relationships(),
		ctx.request.Identity,
		ctx.request.ParentIdentity,
		elemental.OperationRetrieveMany,
	) {
		return makeErrorResponse(
			ctx.ctx,
			response,
			elemental.NewError(
				"Not allowed",
				"RetrieveMany operation not allowed on "+ctx.request.Identity.Category,
				"bahamut",
				http.StatusMethodNotAllowed,
			),
		)
	}

	return runDispatcher(
		ctx,
		response,
		func() error {
			return dispatchRetrieveManyOperation(
				ctx,
				processorFinder,
				cfg.security.requestAuthenticators,
				cfg.security.authorizers,
				pusherFunc,
				cfg.security.auditer,
			)
		},
		cfg.general.panicRecoveryDisabled,
		cfg.opentracing.traceCleaner,
	)
}

func handleRetrieve(ctx *bcontext, cfg config, processorFinder processorFinderFunc, pusherFunc eventPusherFunc) (response *elemental.Response) {

	response = elemental.NewResponse(ctx.request)

	if !elemental.IsOperationAllowed(
		cfg.model.modelManagers[ctx.request.Version].Relationships(),
		ctx.request.Identity,
		ctx.request.ParentIdentity,
		elemental.OperationRetrieve,
	) {
		return makeErrorResponse(
			ctx.ctx,
			response,
			elemental.NewError(
				"Not allowed",
				"Retrieve operation not allowed on "+ctx.request.Identity.Name, "bahamut",
				http.StatusMethodNotAllowed,
			),
		)
	}

	return runDispatcher(
		ctx,
		response,
		func() error {
			return dispatchRetrieveOperation(
				ctx,
				processorFinder,
				cfg.security.requestAuthenticators,
				cfg.security.authorizers,
				pusherFunc,
				cfg.security.auditer,
			)
		},
		cfg.general.panicRecoveryDisabled,
		cfg.opentracing.traceCleaner,
	)
}

func handleCreate(ctx *bcontext, cfg config, processorFinder processorFinderFunc, pusherFunc eventPusherFunc) (response *elemental.Response) {

	response = elemental.NewResponse(ctx.request)

	if !elemental.IsOperationAllowed(
		cfg.model.modelManagers[ctx.request.Version].Relationships(),
		ctx.request.Identity,
		ctx.request.ParentIdentity,
		elemental.OperationCreate,
	) {
		return makeErrorResponse(
			ctx.ctx,
			response,
			elemental.NewError(
				"Not allowed",
				"Create operation not allowed on "+ctx.request.Identity.Name, "bahamut",
				http.StatusMethodNotAllowed,
			),
		)
	}

	return runDispatcher(
		ctx,
		response,
		func() error {
			return dispatchCreateOperation(
				ctx,
				processorFinder,
				cfg.model.modelManagers[ctx.request.Version],
				cfg.model.unmarshallers[ctx.request.Identity],
				cfg.security.requestAuthenticators,
				cfg.security.authorizers,
				pusherFunc,
				cfg.security.auditer,
				cfg.model.readOnly,
				cfg.model.readOnlyExcludedIdentities,
			)
		},
		cfg.general.panicRecoveryDisabled,
		cfg.opentracing.traceCleaner,
	)
}

func handleUpdate(ctx *bcontext, cfg config, processorFinder processorFinderFunc, pusherFunc eventPusherFunc) (response *elemental.Response) {

	response = elemental.NewResponse(ctx.request)

	if !elemental.IsOperationAllowed(
		cfg.model.modelManagers[ctx.request.Version].Relationships(),
		ctx.request.Identity,
		ctx.request.ParentIdentity,
		elemental.OperationUpdate,
	) {
		return makeErrorResponse(
			ctx.ctx,
			response,
			elemental.NewError(
				"Not allowed",
				"Update operation not allowed on "+ctx.request.Identity.Name, "bahamut",
				http.StatusMethodNotAllowed,
			),
		)
	}

	return runDispatcher(
		ctx,
		response,
		func() error {
			return dispatchUpdateOperation(
				ctx,
				processorFinder,
				cfg.model.modelManagers[ctx.request.Version],
				cfg.model.unmarshallers[ctx.request.Identity],
				cfg.security.requestAuthenticators,
				cfg.security.authorizers,
				pusherFunc,
				cfg.security.auditer,
				cfg.model.readOnly,
				cfg.model.readOnlyExcludedIdentities,
			)
		},
		cfg.general.panicRecoveryDisabled,
		cfg.opentracing.traceCleaner,
	)
}

func handleDelete(ctx *bcontext, cfg config, processorFinder processorFinderFunc, pusherFunc eventPusherFunc) (response *elemental.Response) {

	response = elemental.NewResponse(ctx.request)

	if !elemental.IsOperationAllowed(
		cfg.model.modelManagers[ctx.request.Version].Relationships(),
		ctx.request.Identity,
		ctx.request.ParentIdentity,
		elemental.OperationDelete,
	) {
		return makeErrorResponse(
			ctx.ctx,
			response,
			elemental.NewError(
				"Not allowed",
				"Delete operation not allowed on "+ctx.request.Identity.Name, "bahamut",
				http.StatusMethodNotAllowed,
			),
		)
	}

	return runDispatcher(
		ctx,
		response,
		func() error {
			return dispatchDeleteOperation(
				ctx,
				processorFinder,
				cfg.security.requestAuthenticators,
				cfg.security.authorizers,
				pusherFunc,
				cfg.security.auditer,
				cfg.model.readOnly,
				cfg.model.readOnlyExcludedIdentities,
			)
		},
		cfg.general.panicRecoveryDisabled,
		cfg.opentracing.traceCleaner,
	)
}

func handleInfo(ctx *bcontext, cfg config, processorFinder processorFinderFunc, pusherFunc eventPusherFunc) (response *elemental.Response) {

	response = elemental.NewResponse(ctx.request)

	if !elemental.IsOperationAllowed(
		cfg.model.modelManagers[ctx.request.Version].Relationships(),
		ctx.request.Identity,
		ctx.request.ParentIdentity,
		elemental.OperationInfo,
	) {
		return makeErrorResponse(
			ctx.ctx,
			response,
			elemental.NewError(
				"Not allowed",
				"Info operation not allowed on "+ctx.request.Identity.Category, "bahamut",
				http.StatusMethodNotAllowed,
			),
		)
	}

	return runDispatcher(
		ctx,
		response,
		func() error {
			return dispatchInfoOperation(
				ctx,
				processorFinder,
				cfg.security.requestAuthenticators,
				cfg.security.authorizers,
				pusherFunc,
				cfg.security.auditer,
			)
		},
		cfg.general.panicRecoveryDisabled,
		cfg.opentracing.traceCleaner,
	)
}

func handlePatch(ctx *bcontext, cfg config, processorFinder processorFinderFunc, pusherFunc eventPusherFunc) (response *elemental.Response) {

	response = elemental.NewResponse(ctx.request)

	if !elemental.IsOperationAllowed(
		cfg.model.modelManagers[ctx.request.Version].Relationships(),
		ctx.request.Identity,
		ctx.request.ParentIdentity,
		elemental.OperationPatch,
	) {
		return makeErrorResponse(
			ctx.ctx,
			response,
			elemental.NewError(
				"Not allowed",
				"Patch operation not allowed on "+ctx.request.Identity.Category, "bahamut",
				http.StatusMethodNotAllowed,
			),
		)
	}

	return runDispatcher(
		ctx,
		response,
		func() error {
			return dispatchPatchOperation(
				ctx,
				processorFinder,
				cfg.model.modelManagers[ctx.request.Version],
				cfg.model.unmarshallers[ctx.request.Identity],
				cfg.security.requestAuthenticators,
				cfg.security.authorizers,
				pusherFunc,
				cfg.security.auditer,
				cfg.model.readOnly,
				cfg.model.readOnlyExcludedIdentities,
			)
		},
		cfg.general.panicRecoveryDisabled,
		cfg.opentracing.traceCleaner,
	)
}
