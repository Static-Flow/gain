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

package gain

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"

	"github.com/pawelgaczynski/gain/iouring"
	"github.com/pawelgaczynski/gain/pkg/errors"
	"github.com/pawelgaczynski/gain/pkg/socket"
)

type acceptor struct {
	ring              *iouring.Ring
	fd                int
	clientAddr        *syscall.RawSockaddrAny
	clientLenPointer  *uint32
	connectionManager *connectionManager
}

func (a *acceptor) addAcceptRequest() error {
	entry, err := a.ring.GetSQE()
	if err != nil {
		return fmt.Errorf("error getting SQE: %w", err)
	}

	entry.PrepareAccept(
		a.fd, uintptr(unsafe.Pointer(a.clientAddr)), uint64(uintptr(unsafe.Pointer(a.clientLenPointer))), 0)
	entry.UserData = acceptDataFlag | uint64(a.fd)

	return nil
}

func (a *acceptor) addAcceptConnRequest() error {
	err := a.addAcceptRequest()
	if err != nil {
		return err
	}

	conn := a.connectionManager.getFd(a.fd)
	if conn == nil {
		return errors.ErrConnectionIsMissing
	}
	conn.state = connAccept

	return nil
}

func (a *acceptor) lastClientAddr() (net.Addr, error) {
	addr, err := anyToSockaddr(a.clientAddr)
	if err != nil {
		return nil, err
	}

	return socket.SockaddrToTCPOrUnixAddr(addr), nil
}

func newAcceptor(ring *iouring.Ring, connectionManager *connectionManager) *acceptor {
	acceptor := &acceptor{
		ring:              ring,
		connectionManager: connectionManager,
	}
	acceptor.clientAddr, acceptor.clientLenPointer = createClientAddr()

	return acceptor
}
