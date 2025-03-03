package gain

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

import (
	"fmt"
	"net"
	"sync"
	"syscall"

	"github.com/pawelgaczynski/gain/iouring"
	"github.com/pawelgaczynski/gain/logger"
	gainErrors "github.com/pawelgaczynski/gain/pkg/errors"
	"github.com/pawelgaczynski/gain/pkg/queue"
)

type consumerConfig struct {
	readWriteWorkerConfig
}

type consumer interface {
	readWriteWorker
	addConnToQueue(fd int) error
	setSocketAddr(fd int, addr net.Addr)
}

type consumerWorker struct {
	*readWriteWorkerImpl
	config consumerConfig

	socketAddresses sync.Map
	// used for kernels < 5.18 where OP_MSG_RING is not supported
	connQueue queue.LockFreeQueue[int]
}

func (c *consumerWorker) setSocketAddr(fd int, addr net.Addr) {
	c.socketAddresses.Store(fd, addr)
}

func (c *consumerWorker) addConnToQueue(fd int) error {
	if c.connQueue == nil {
		return gainErrors.ErrConnectionQueueIsNil
	}

	c.connQueue.Enqueue(fd)

	return nil
}

func (c *consumerWorker) closeAllConns() {
	c.logWarn().Msg("Closing connections")
	c.connectionManager.close(func(conn *connection) bool {
		err := c.addCloseConnRequest(conn)
		if err != nil {
			c.logError(err).Msg("Add close() connection request error")
		}

		return err == nil
	}, -1)
}

func (c *consumerWorker) activeConnections() int {
	return c.connectionManager.activeConnections(func(c *connection) bool {
		return true
	})
}

func (c *consumerWorker) handleConn(conn *connection, cqe *iouring.CompletionQueueEvent) {
	var (
		err    error
		errMsg string
	)

	switch conn.state {
	case connRead:
		err = c.onRead(cqe, conn)
		if err != nil {
			errMsg = "read error"
		}

	case connWrite:
		n := int(cqe.Res())
		conn.onKernelWrite(n)
		c.logDebug().Int("fd", conn.fd).Int32("count", cqe.Res()).Msg("Bytes writed")

		conn.setUserSpace()
		c.eventHandler.OnWrite(conn, n)

		err = c.addNextRequest(conn)
		if err != nil {
			errMsg = "add request error"
		}

	case connClose:
		if cqe.UserData()&closeConnFlag > 0 {
			c.closeConn(conn, false, nil)
		} else if cqe.UserData()&writeDataFlag > 0 {
			n := int(cqe.Res())
			conn.onKernelWrite(n)
			c.logDebug().Int("fd", conn.fd).Int32("count", cqe.Res()).Msg("Bytes writed")
			conn.setUserSpace()
			c.eventHandler.OnWrite(conn, n)
		}

	default:
		err = gainErrors.ErrorUnknownConnectionState(int(conn.state))
	}

	if err != nil {
		c.logError(err).Msg(errMsg)
		c.closeConn(conn, true, err)
	}
}

func (c *consumerWorker) getConnsFromQueue() {
	for {
		if c.connQueue.IsEmpty() {
			break
		}
		fd := c.connQueue.Dequeue()

		conn := c.connectionManager.getFd(fd)
		if conn == nil {
			c.logError(gainErrors.ErrorConnectionIsMissing(fd)).Msg("Get new connection error")

			continue
		}
		conn.fd = fd
		conn.localAddr = c.localAddr

		if remoteAddr, ok := c.socketAddresses.Load(fd); ok {
			conn.remoteAddr, _ = remoteAddr.(net.Addr)

			c.socketAddresses.Delete(fd)
		} else {
			c.logError(gainErrors.ErrorAddressNotFound(fd)).Msg("Get new connection error")
		}

		conn.setUserSpace()
		c.eventHandler.OnAccept(conn)

		err := c.addNextRequest(conn)
		if err != nil {
			c.logError(err).Msg("add request error")
		}
	}
}

func (c *consumerWorker) handleJobsInQueues() {
	if c.connQueue != nil {
		c.getConnsFromQueue()
	}

	c.handleAsyncWritesIfEnabled()
}

func (c *consumerWorker) loop(_ int) error {
	c.logInfo().Msg("Starting consumer loop...")
	c.prepareHandler = func() error {
		c.startedChan <- done

		return nil
	}
	c.shutdownHandler = func() bool {
		if c.needToShutdown() {
			c.onCloseHandler()
			c.markShutdownInProgress()
		}

		return true
	}
	c.loopFinisher = c.handleJobsInQueues
	c.loopFinishCondition = func() bool {
		if c.connectionManager.allClosed() {
			c.notifyFinish()

			return true
		}

		return false
	}

	return c.looper.startLoop(c.index(), func(cqe *iouring.CompletionQueueEvent) error {
		if exit := c.processEvent(cqe, func(cqe *iouring.CompletionQueueEvent) bool {
			keyOrFd := cqe.UserData() & ^allFlagsMask

			return c.connectionManager.get(int(keyOrFd), 0) == nil
		}); exit {
			return nil
		}
		if cqe.UserData()&addConnFlag > 0 {
			fileDescriptor := int(cqe.Res())
			conn := c.connectionManager.getFd(fileDescriptor)
			if conn == nil {
				c.logError(gainErrors.ErrorConnectionIsMissing(fileDescriptor)).Msg("Get new connection error")
				_ = c.syscallCloseSocket(fileDescriptor)

				return nil
			}
			conn.fd = int(cqe.Res())
			conn.localAddr = c.localAddr
			if remoteAddr, ok := c.socketAddresses.Load(conn.fd); ok {
				conn.remoteAddr, _ = remoteAddr.(net.Addr)
				c.socketAddresses.Delete(conn.fd)
			} else {
				c.logError(gainErrors.ErrorAddressNotFound(conn.fd)).Msg("Get new connection error")
			}

			conn.setUserSpace()
			c.eventHandler.OnAccept(conn)

			return c.addNextRequest(conn)
		}
		fileDescriptor := int(cqe.UserData() & ^allFlagsMask)
		if fileDescriptor < syscall.Stderr {
			c.logError(nil).Int("fd", fileDescriptor).Msg("Invalid file descriptor")

			return nil
		}
		conn := c.connectionManager.getFd(fileDescriptor)
		if conn == nil {
			c.logError(gainErrors.ErrorConnectionIsMissing(fileDescriptor)).Msg("Get connection error")
			_ = c.syscallCloseSocket(fileDescriptor)

			return nil
		}
		c.handleConn(conn, cqe)

		return nil
	})
}

func newConsumerWorker(
	index int, localAddr net.Addr, config consumerConfig, eventHandler EventHandler, features supportedFeatures,
) (*consumerWorker, error) {
	ring, err := iouring.CreateRing()
	if err != nil {
		return nil, fmt.Errorf("creating ring error: %w", err)
	}
	logger := logger.NewLogger("consumer", config.loggerLevel, config.prettyLogger)
	connectionManager := newConnectionManager()
	consumer := &consumerWorker{
		config: config,
		readWriteWorkerImpl: newReadWriteWorkerImpl(
			ring, index, localAddr, eventHandler, connectionManager, config.readWriteWorkerConfig, logger,
		),
	}

	if !features.ringsMessaging {
		consumer.connQueue = queue.NewIntQueue()
	}
	consumer.onCloseHandler = consumer.closeAllConns

	return consumer, nil
}
