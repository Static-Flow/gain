// Copyright (c) 2023 Paweł Gaczyński
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

package iouring

import (
	"errors"
	"fmt"
)

var (
	ErrNotImplemented     = errors.New("not implemented")
	ErrNotSupported       = errors.New("not supported")
	ErrTimerExpired       = errors.New("timer expired")
	ErrInterrupredSyscall = errors.New("interrupred system call")
	ErrAgain              = errors.New("try again")
	ErrSQEOverflow        = errors.New("SQE overflow")
)

func ErrorSQEOverflow(overflowValue uint32) error {
	return fmt.Errorf("%w: %d", ErrSQEOverflow, overflowValue)
}
