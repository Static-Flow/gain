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
	"net"
	"testing"

	. "github.com/stretchr/testify/require"
)

const numberOfTestWorkers = 4

type testWorker struct {
	conns int
}

func (w *testWorker) activeConnections() int {
	return w.conns
}

func (w *testWorker) setIndex(_ int) {
}

func (w *testWorker) index() int {
	return 0
}

func (w *testWorker) loop(_ int) error {
	w.conns++

	return nil
}

func (w *testWorker) shutdown() {
}

func (w *testWorker) setSocketAddr(_ int, _ net.Addr) {
}

func (w *testWorker) addConnToQueue(_ int) error {
	return nil
}

func (w *testWorker) ringFd() int {
	return 0
}

func (w *testWorker) started() bool {
	return true
}

func createTestWorkers() []*testWorker {
	workers := make([]*testWorker, 0)
	for i := 0; i < numberOfTestWorkers; i++ {
		workers = append(workers, &testWorker{})
	}

	return workers
}

func TestRoundRobinLoadBalander(t *testing.T) {
	lb := newRoundRobinLoadBalancer()

	workers := createTestWorkers()
	for _, worker := range workers {
		lb.register(worker)
	}
	worker := lb.next(nil)
	err := worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[0])
	worker = lb.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[1])
	worker = lb.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[2])
	worker = lb.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[3])
	worker = lb.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[0])
	worker = lb.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[1])
	worker = lb.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[2])
	worker = lb.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[3])
}

func TestLeastConnectionsLoadBalander(t *testing.T) {
	loadBl := newLeastConnectionsLoadBalancer()

	workers := createTestWorkers()
	for _, worker := range workers {
		loadBl.register(worker)
	}
	workers[0].conns = 1
	workers[1].conns = 0
	workers[2].conns = 2
	workers[3].conns = 1
	worker := loadBl.next(nil)
	err := worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[1])
	worker = loadBl.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[0])
	worker = loadBl.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[1])
	worker = loadBl.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[3])
	worker = loadBl.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[0])
	worker = loadBl.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[1])
	worker = loadBl.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[2])
	worker = loadBl.next(nil)
	err = worker.loop(0)
	Nil(t, err)
	Same(t, worker, workers[3])
}
