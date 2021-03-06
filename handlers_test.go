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
	"testing"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	. "github.com/smartystreets/goconvey/convey"
	"go.aporeto.io/elemental"
	testmodel "go.aporeto.io/elemental/test/model"
)

func TestHandlers_makeResponse(t *testing.T) {

	Convey("Given I have context with a redirect and a response", t, func() {

		ctx := newContext(context.TODO(), elemental.NewRequest())
		response := elemental.NewResponse(elemental.NewRequest())

		ctx.redirect = "http://ici"

		Convey("When I call makeResponse", func() {

			makeResponse(ctx, response, nil)

			Convey("Then response.Redirect should be set", func() {
				So(response.Redirect, ShouldEqual, "http://ici")
			})
		})
	})

	Convey("Given I have context with a a count and a response", t, func() {

		ctx := newContext(context.TODO(), elemental.NewRequest())
		response := elemental.NewResponse(elemental.NewRequest())

		ctx.count = 42

		Convey("When I call makeResponse on a retrieveMany operation", func() {

			ctx.request.Operation = elemental.OperationRetrieveMany

			makeResponse(ctx, response, nil)

			Convey("Then response.Total should be set", func() {
				So(response.Total, ShouldEqual, 42)
			})
		})

		Convey("When I call makeResponse on a info operation", func() {

			ctx.request.Operation = elemental.OperationInfo

			makeResponse(ctx, response, nil)

			Convey("Then response.Total should be set", func() {
				So(response.Total, ShouldEqual, 42)
			})
		})

		Convey("When I call makeResponse on a create operation", func() {

			ctx.request.Operation = elemental.OperationCreate

			makeResponse(ctx, response, nil)

			Convey("Then response.Total should not be set", func() {
				So(response.Total, ShouldEqual, 0)
			})
		})

		Convey("When I call makeResponse on a update operation", func() {

			ctx.request.Operation = elemental.OperationUpdate

			makeResponse(ctx, response, nil)

			Convey("Then response.Total should not be set", func() {
				So(response.Total, ShouldEqual, 0)
			})
		})

		Convey("When I call makeResponse on a delete operation", func() {

			ctx.request.Operation = elemental.OperationDelete

			makeResponse(ctx, response, nil)

			Convey("Then response.Total should not be set", func() {
				So(response.Total, ShouldEqual, 0)
			})
		})

		Convey("When I call makeResponse on a patch operation", func() {

			ctx.request.Operation = elemental.OperationPatch

			makeResponse(ctx, response, nil)

			Convey("Then response.Total should not be set", func() {
				So(response.Total, ShouldEqual, 0)
			})
		})
	})

	Convey("Given I have context with a status code set to 0 and a response", t, func() {

		ctx := newContext(context.TODO(), elemental.NewRequest())
		response := elemental.NewResponse(elemental.NewRequest())
		ctx.outputData = []string{}
		ctx.statusCode = 0

		Convey("When I set the operation to Create and I call makeResponse", func() {

			ctx.request.Operation = elemental.OperationCreate

			makeResponse(ctx, response, nil)

			Convey("Then response.StatusCode should equal", func() {
				So(response.StatusCode, ShouldEqual, http.StatusCreated)
			})
		})

		Convey("When I set the operation to Info and I call makeResponse", func() {

			ctx.request.Operation = elemental.OperationInfo

			makeResponse(ctx, response, nil)

			Convey("Then response.StatusCode should equal", func() {
				So(response.StatusCode, ShouldEqual, http.StatusNoContent)
			})
		})

		Convey("When I set the operation to Retrieve and I call makeResponse", func() {

			ctx.request.Operation = elemental.OperationRetrieve

			makeResponse(ctx, response, nil)

			Convey("Then response.StatusCode should equal", func() {
				So(response.StatusCode, ShouldEqual, http.StatusOK)
			})
		})

		Convey("When I set the operation to Create, status code created, but no data, and I call makeResponse", func() {

			ctx.request.Operation = elemental.OperationCreate
			ctx.statusCode = http.StatusCreated
			ctx.outputData = nil

			makeResponse(ctx, response, nil)

			Convey("Then response.StatusCode should equal", func() {
				So(response.StatusCode, ShouldEqual, http.StatusNoContent)
			})
		})
	})

	Convey("Given I have context with messages and a response", t, func() {

		ctx := newContext(context.TODO(), elemental.NewRequest())
		response := elemental.NewResponse(elemental.NewRequest())
		ctx.AddMessage("hello world")

		Convey("When I call makeResponse", func() {

			makeResponse(ctx, response, nil)

			Convey("Then response.Message should be set", func() {
				So(response.Messages, ShouldResemble, []string{"hello world"})
			})
		})
	})

	Convey("Given I have context indentifiable output data and a cleaner func", t, func() {

		req := elemental.NewRequest()
		req.Headers.Add("X-Fields", "name")
		req.Headers.Add("X-Fields", "ID")

		tracer := &mockTracer{}
		ts := newMockSpan(tracer)
		tctx := opentracing.ContextWithSpan(context.Background(), ts)

		ctx := newContext(tctx, req)

		response := elemental.NewResponse(req)
		ctx.outputData = &testmodel.List{
			Name:        "the name",
			ID:          "xxx",
			Description: " the description",
		}

		Convey("When I call makeResponse", func() {

			resp := makeResponse(ctx, response, func(identity elemental.Identity, data []byte) []byte {
				return []byte("modified")
			})

			Convey("Then output data should be correct", func() {
				So(string(resp.Data), ShouldEqual, `{"ID":"xxx","name":"the name"}`)
			})

			Convey("Then span content should be modified", func() {
				So(ts.fields[0].String(), ShouldEqual, "response:modified")
			})
		})
	})

	Convey("Given I have context with unmarshalable data and a response", t, func() {

		ctx := newContext(context.TODO(), elemental.NewRequest())
		response := elemental.NewResponse(elemental.NewRequest())
		ctx.outputData = testmodel.NewUnmarshalableList()

		Convey("When I call makeResponse", func() {

			Convey("Then it should panic", func() {
				So(func() { makeResponse(ctx, response, nil) }, ShouldPanic)
			})
		})
	})

	Convey("Given I have context with X-Fields header and indentifiable output data", t, func() {

		req := elemental.NewRequest()
		req.Headers.Add("X-Fields", "name")
		req.Headers.Add("X-Fields", "ID")

		ctx := newContext(context.TODO(), req)
		response := elemental.NewResponse(req)
		ctx.outputData = &testmodel.List{
			Name:        "the name",
			ID:          "xxx",
			Description: " the description",
		}

		Convey("When I call makeResponse", func() {

			resp := makeResponse(ctx, response, nil)

			Convey("Then output data should be correct", func() {
				So(string(resp.Data), ShouldEqual, `{"ID":"xxx","name":"the name"}`)
			})
		})
	})

	Convey("Given I have context with X-Fields header and indentifiable output data", t, func() {

		req := elemental.NewRequest()
		req.Headers.Add("X-Fields", "name")
		req.Headers.Add("X-Fields", "ID")

		ctx := newContext(context.TODO(), req)
		response := elemental.NewResponse(req)
		ctx.outputData = testmodel.ListsList{
			&testmodel.List{
				Name:        "the name",
				ID:          "xxx",
				Description: " the description",
			},
			&testmodel.List{
				Name:        "the name2",
				ID:          "xxx2",
				Description: " the description2",
			},
		}

		Convey("When I call makeResponse", func() {

			resp := makeResponse(ctx, response, nil)

			Convey("Then output data should be correct", func() {
				So(string(resp.Data), ShouldEqual, `[{"ID":"xxx","name":"the name"},{"ID":"xxx2","name":"the name2"}]`)
			})
		})
	})
}

func TestHandlers_makeErrorResponse(t *testing.T) {

	Convey("Given I a response and an error", t, func() {

		resp := elemental.NewResponse(elemental.NewRequest())
		err := fmt.Errorf("paf")

		Convey("When I call makeErrorResponse", func() {

			r := makeErrorResponse(context.Background(), resp, err)

			Convey("Then the returned response should be the same", func() {
				So(resp, ShouldEqual, r)
			})

			Convey("Then response should be correctly encoded", func() {
				So(string(resp.Data), ShouldEqual, `[{"code":500,"data":null,"description":"paf","subject":"elemental","title":"Internal Server Error","trace":"unknown"}]`)
			})
		})
	})

	Convey("Given I a response and an context canceled error", t, func() {

		err := context.Canceled

		Convey("When I call makeErrorResponse", func() {

			r := makeErrorResponse(context.Background(), nil, err)

			Convey("Then the returned response should be the same", func() {
				So(r, ShouldEqual, nil)
			})
		})
	})
}

func TestHandlers_handleEventualPanic(t *testing.T) {

	Convey("Given I have a response and a channel", t, func() {

		c := make(chan error)

		Convey("When I call my function that panics with handleEventualPanic installed with recover", func() {

			f := func() {
				defer handleEventualPanic(context.Background(), c, false)
				panic("Noooooooooooooooooo")
			}

			go f()

			err := <-c

			Convey("Then err should be correct", func() {
				So(err.Error(), ShouldEqual, "error 500 (bahamut): Internal Server Error: Noooooooooooooooooo")
			})
		})

		Convey("When I call my function that panics with handleEventualPanic installed with no recover", func() {

			f := func() {
				defer handleEventualPanic(context.Background(), c, true)
				panic("Noooooooooooooooooo")
			}

			Convey("Then it should panic", func() {
				So(f, ShouldPanic)
			})
		})
	})
}

func TestHandlers_runDispatcher(t *testing.T) {

	Convey("Given I have a fake dispatcher", t, func() {

		calledCounter := &counter{}

		gctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		ctx := newContext(context.TODO(), elemental.NewRequest())
		ctx.request = elemental.NewRequest()
		ctx.ctx = gctx

		response := elemental.NewResponse(elemental.NewRequest())

		Convey("When I call runDispatcher", func() {

			d := func() error {
				calledCounter.Add(1)
				return nil
			}

			r := runDispatcher(ctx, response, d, true, nil)

			Convey("Then the code should be 204", func() {
				So(r.StatusCode, ShouldEqual, 204)
			})

			Convey("Then the dispatcher should have been called once", func() {
				So(calledCounter.Value(), ShouldEqual, 1)
			})
		})

		Convey("When I call runDispatcher and it returns an error", func() {

			d := func() error {
				calledCounter.Add(1)
				return elemental.NewError("nop", "nope", "test", 42)
			}

			r := runDispatcher(ctx, response, d, true, nil)

			Convey("Then the code should be 42", func() {
				So(r.StatusCode, ShouldEqual, 42)
			})

			Convey("Then the dispatcher should have been called once", func() {
				So(calledCounter.Value(), ShouldEqual, 1)
			})
		})

		Convey("When I call runDispatcher and cancel the context", func() {

			d := func() error {
				time.Sleep(300 * time.Millisecond)
				calledCounter.Add(1)
				return nil
			}

			r := elemental.NewResponse(elemental.NewRequest())
			go func() { runDispatcher(ctx, r, d, true, nil) }()
			time.Sleep(30 * time.Millisecond)
			cancel()

			Convey("Then the dispatcher should have been called once", func() {
				So(calledCounter.Value(), ShouldEqual, 0)
			})
		})
	})
}

func TestHandlers_handleRetrieveMany(t *testing.T) {

	Convey("Given I have a config", t, func() {

		cfg := config{}
		cfg.model.modelManagers = map[int]elemental.ModelManager{
			0: testmodel.Manager(),
			1: testmodel.Manager(),
		}

		Convey("Given I a have fake processor finder that return no error", func() {

			calledCounter := &counter{}
			pf := func(identity elemental.Identity) (Processor, error) {
				calledCounter.Add(1)
				return struct{}{}, nil
			}

			Convey("When I call handleRetrieveMany on valid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.ParentIdentity = elemental.RootIdentity
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationRetrieveMany
				ctx.statusCode = http.StatusAccepted

				resp := handleRetrieveMany(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":501,"data":null,"description":"No handler for operation retrieve-many on user","subject":"bahamut","title":"Not implemented","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 1)
				})
			})

			Convey("When I call handleRetrieveMany on invalid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationRetrieveMany
				ctx.request.ParentIdentity = testmodel.UserIdentity

				resp := handleRetrieveMany(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":405,"data":null,"description":"RetrieveMany operation not allowed on users","subject":"bahamut","title":"Not allowed","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 0)
				})
			})
		})
	})
}

func TestHandlers_handleRetrieve(t *testing.T) {

	Convey("Given I have a config", t, func() {

		cfg := config{}
		cfg.model.modelManagers = map[int]elemental.ModelManager{
			0: testmodel.Manager(),
			1: testmodel.Manager(),
		}

		Convey("Given I a have fake processor finder that return no error", func() {

			calledCounter := &counter{}
			pf := func(identity elemental.Identity) (Processor, error) {
				calledCounter.Add(1)
				return struct{}{}, nil
			}

			Convey("When I call handleRetrieve on valid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationRetrieve
				ctx.statusCode = http.StatusAccepted
				ctx.request.ParentIdentity = elemental.RootIdentity

				resp := handleRetrieve(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":501,"data":null,"description":"No handler for operation retrieve on user","subject":"bahamut","title":"Not implemented","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 1)
				})
			})

			Convey("When I call handleRetrieve on invalid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationRetrieve
				ctx.request.ParentIdentity = testmodel.UserIdentity

				resp := handleRetrieve(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":405,"data":null,"description":"Retrieve operation not allowed on user","subject":"bahamut","title":"Not allowed","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 0)
				})
			})
		})
	})
}

func TestHandlers_handleCreate(t *testing.T) {

	Convey("Given I have a config", t, func() {

		cfg := config{}
		cfg.model.modelManagers = map[int]elemental.ModelManager{
			0: testmodel.Manager(),
			1: testmodel.Manager(),
		}

		Convey("Given I a have fake processor finder that return no error", func() {

			calledCounter := &counter{}
			pf := func(identity elemental.Identity) (Processor, error) {
				calledCounter.Add(1)
				return struct{}{}, nil
			}

			Convey("When I call handleCreate on valid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationCreate
				ctx.statusCode = http.StatusAccepted
				ctx.request.ParentIdentity = elemental.RootIdentity

				resp := handleCreate(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":501,"data":null,"description":"No handler for operation create on user","subject":"bahamut","title":"Not implemented","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 1)
				})
			})

			Convey("When I call handleCreate on invalid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationCreate
				ctx.request.ParentIdentity = testmodel.UserIdentity

				resp := handleCreate(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":405,"data":null,"description":"Create operation not allowed on user","subject":"bahamut","title":"Not allowed","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 0)
				})
			})
		})
	})
}

func TestHandlers_handleUpdate(t *testing.T) {

	Convey("Given I have a config", t, func() {

		cfg := config{}
		cfg.model.modelManagers = map[int]elemental.ModelManager{
			0: testmodel.Manager(),
			1: testmodel.Manager(),
		}

		Convey("Given I a have fake processor finder that return no error", func() {

			calledCounter := &counter{}
			pf := func(identity elemental.Identity) (Processor, error) {
				calledCounter.Add(1)
				return struct{}{}, nil
			}

			Convey("When I call handleUpdate on valid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationUpdate
				ctx.statusCode = http.StatusAccepted
				ctx.request.ParentIdentity = elemental.RootIdentity

				resp := handleUpdate(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":501,"data":null,"description":"No handler for operation update on user","subject":"bahamut","title":"Not implemented","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 1)
				})
			})

			Convey("When I call handleUpdate on invalid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationUpdate
				ctx.request.ParentIdentity = testmodel.UserIdentity

				resp := handleUpdate(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":405,"data":null,"description":"Update operation not allowed on user","subject":"bahamut","title":"Not allowed","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 0)
				})
			})
		})
	})
}

func TestHandlers_handleDelete(t *testing.T) {

	Convey("Given I have a config", t, func() {

		cfg := config{}
		cfg.model.modelManagers = map[int]elemental.ModelManager{
			0: testmodel.Manager(),
			1: testmodel.Manager(),
		}

		Convey("Given I a have fake processor finder that return no error", func() {

			calledCounter := &counter{}
			pf := func(identity elemental.Identity) (Processor, error) {
				calledCounter.Add(1)
				return struct{}{}, nil
			}

			Convey("When I call handleDelete on valid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationDelete
				ctx.statusCode = http.StatusAccepted
				ctx.request.ParentIdentity = elemental.RootIdentity

				resp := handleDelete(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":501,"data":null,"description":"No handler for operation delete on user","subject":"bahamut","title":"Not implemented","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 1)
				})
			})

			Convey("When I call handleDelete on invalid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationDelete
				ctx.request.ParentIdentity = testmodel.UserIdentity

				resp := handleDelete(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":405,"data":null,"description":"Delete operation not allowed on user","subject":"bahamut","title":"Not allowed","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 0)
				})
			})
		})
	})
}

func TestHandlers_handleInfo(t *testing.T) {

	Convey("Given I have a config", t, func() {

		cfg := config{}
		cfg.model.modelManagers = map[int]elemental.ModelManager{
			0: testmodel.Manager(),
			1: testmodel.Manager(),
		}

		Convey("Given I a have fake processor finder that return no error", func() {

			calledCounter := &counter{}
			pf := func(identity elemental.Identity) (Processor, error) {
				calledCounter.Add(1)
				return struct{}{}, nil
			}

			Convey("When I call handleInfo on valid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationInfo
				ctx.statusCode = http.StatusAccepted
				ctx.request.ParentIdentity = elemental.RootIdentity

				resp := handleInfo(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":501,"data":null,"description":"No handler for operation info on user","subject":"bahamut","title":"Not implemented","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 1)
				})
			})

			Convey("When I call handleInfo on invalid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationInfo
				ctx.request.ParentIdentity = testmodel.UserIdentity

				resp := handleInfo(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":405,"data":null,"description":"Info operation not allowed on users","subject":"bahamut","title":"Not allowed","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 0)
				})
			})
		})
	})
}

func TestHandlers_handlePatch(t *testing.T) {

	Convey("Given I have a config", t, func() {

		cfg := config{}
		cfg.model.modelManagers = map[int]elemental.ModelManager{
			0: testmodel.Manager(),
			1: testmodel.Manager(),
		}

		Convey("Given I a have fake processor finder that return no error", func() {

			calledCounter := &counter{}
			pf := func(identity elemental.Identity) (Processor, error) {
				calledCounter.Add(1)
				return struct{}{}, nil
			}

			Convey("When I call handlePatch on valid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationPatch
				ctx.statusCode = http.StatusAccepted
				ctx.request.ParentIdentity = elemental.RootIdentity

				resp := handlePatch(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":501,"data":null,"description":"No handler for operation patch on user","subject":"bahamut","title":"Not implemented","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 1)
				})
			})

			Convey("When I call handlePatch on invalid api call", func() {

				ctx := newContext(context.TODO(), elemental.NewRequest())
				ctx.request = elemental.NewRequest()
				ctx.request.Identity = testmodel.UserIdentity
				ctx.request.Operation = elemental.OperationPatch
				ctx.request.ParentIdentity = testmodel.UserIdentity

				resp := handlePatch(ctx, cfg, pf, nil)

				Convey("Then resp should be correct", func() {
					So(resp, ShouldNotBeNil)
					So(string(resp.Data), ShouldEqual, `[{"code":405,"data":null,"description":"Patch operation not allowed on users","subject":"bahamut","title":"Not allowed","trace":"unknown"}]`)
				})

				Convey("Then the dispactcher should have been called once", func() {
					So(calledCounter.Value(), ShouldEqual, 0)
				})
			})
		})
	})
}
