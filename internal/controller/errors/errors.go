// Copyright 2022 Linkall Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package errors

import rpcerr "github.com/linkall-labs/vsproto/pkg/errors"

var (
	ErrInvalidRequest       = rpcerr.New("invalid request").WithGRPCCode(rpcerr.ErrorCode_INVALID_REQUEST)
	ErrResourceNotFound     = rpcerr.New("resource not found").WithGRPCCode(rpcerr.ErrorCode_RESOURCE_NOT_FOUND)
	ErrResourceAlreadyExist = rpcerr.New("resource already exist").WithGRPCCode(rpcerr.ErrorCode_RESOURCE_EXIST)

	ErrServerNotStart = rpcerr.New("server not start").WithGRPCCode(rpcerr.ErrorCode_SERVICE_NOT_RUNNING)
	ErrJsonMarshal    = rpcerr.New("json marshal").WithGRPCCode(rpcerr.ErrorCode_INTERNAL)
	ErrJsonUnMarshal  = rpcerr.New("json unmarshal").WithGRPCCode(rpcerr.ErrorCode_INTERNAL)

	ErrTriggerWorker = rpcerr.New("trigger worker error").WithGRPCCode(rpcerr.ErrorCode_INTERNAL)

	ErrCeSqlExpression        = rpcerr.New("ce sql expression invalid").WithGRPCCode(rpcerr.ErrorCode_INVALID_REQUEST)
	ErrFilterAttributeIsEmpty = rpcerr.New("filter dialect attribute is empty").WithGRPCCode(rpcerr.ErrorCode_INVALID_REQUEST)
	ErrFilterMultiple         = rpcerr.New("filter multiple dialects found").WithGRPCCode(rpcerr.ErrorCode_INVALID_REQUEST)
)