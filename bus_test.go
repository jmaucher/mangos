// Copyright 2014 Garrett D'Amore
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use file except in compliance with the License.
// You may obtain a copy of the license at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sp

import (
	"math/rand"
	"testing"
	"time"
)

type busTester struct {
	id     int
	sock   Socket
	rdoneq chan bool
	sdoneq chan bool
}

func busTestSender(t *testing.T, bt *busTester, cnt int) {
	defer close(bt.sdoneq)
	for i := 0; i < cnt; i++ {
		// Inject a small delay to give receivers a chance to catch up
		// Maximum is 10 msec.
		d := time.Duration(rand.Uint32() % 10000)
		time.Sleep(d * time.Microsecond)
		t.Logf("Peer %d: Sending %d", bt.id, i)
		msg := NewMessage(2)
		msg.Body = append(msg.Body, byte(bt.id), byte(i))
		if err := bt.sock.SendMsg(msg); err != nil {
			t.Errorf("Peer %d send %d fail: %v", bt.id, i, err)
			return
		}
	}
}

func busTestReceiver(t *testing.T, bt *busTester, cnt int, numId int) {
	var rcpt = make([]int, numId)
	defer close(bt.rdoneq)

	for tot := 0; tot < (numId-1)*cnt; {
		msg, err := bt.sock.RecvMsg()
		if err != nil {
			t.Errorf("Peer %d: Recv fail: %v", bt.id, err)
			return
		}

		if len(msg.Body) != 2 {
			t.Errorf("Peer %d: Received wrong length", bt.id)
			return
		}
		peer := int(msg.Body[0])
		if peer == bt.id {
			t.Errorf("Peer %d: Got its own message!", bt.id)
			return
		}
		if int(msg.Body[1]) != rcpt[peer] {
			t.Errorf("Peer %d: Wrong message from peer %d: %d",
				bt.id, peer, msg.Body[1])
			return
		}
		if int(msg.Body[1]) >= cnt {
			t.Errorf("Peer %d: Too many from peer %d", bt.id,
				peer)
			return
		}
		t.Logf("Peer %d: Good rcv from peer %d (%d)", bt.id, peer,
			rcpt[peer])
		rcpt[peer]++
		tot++
		msg.Free()
	}
	t.Logf("Peer %d: Finish", bt.id)
}

func busTestNewServer(t *testing.T, addr string, id int) *busTester {
	var err error
	bt := &busTester{id: id, rdoneq: make(chan bool), sdoneq: make(chan bool)}

	if bt.sock, err = NewSocket(BusName); err != nil {
		t.Errorf("Failed getting server %d socket: %v", id, err)
		return nil
	}

	if err = bt.sock.Listen(addr); err != nil {
		t.Errorf("Failed server %d listening: %v", id, err)
		bt.sock.Close()
		return nil
	}
	return bt
}

func busTestNewClient(t *testing.T, addr string, id int) *busTester {
	var err error
	bt := &busTester{id: id, rdoneq: make(chan bool), sdoneq: make(chan bool)}

	if bt.sock, err = NewSocket(BusName); err != nil {
		t.Errorf("Failed getting client %d socket: %v", id, err)
		return nil
	}
	if err = bt.sock.Dial(addr); err != nil {
		t.Errorf("Failed client %d dialing: %v", id, err)
		bt.sock.Close()
		return nil
	}
	return bt
}

func busTestCleanup(t *testing.T, bts []*busTester) {
	for id := 0; id < len(bts); id++ {
		t.Logf("Cleanup %d", id)
		if bts[id].sock != nil {
			bts[id].sock.Close()
		}
	}
}

func TestBus(t *testing.T) {
	addr := "tcp://127.0.0.1:3538"

	num := 5
	pkts := 7
	bts := make([]*busTester, num)
	defer busTestCleanup(t, bts)

	t.Logf("Creating bus network")
	for id := 0; id < num; id++ {
		if id == 0 {
			bts[id] = busTestNewServer(t, addr, id)
		} else {
			bts[id] = busTestNewClient(t, addr, id)
		}
		if bts[id] == nil {
			t.Errorf("Failed creating %d", id)
			return
		}
	}

	// wait a little bit for connections to establish
	time.Sleep(time.Microsecond * 500)

	t.Logf("Starting send/recv")
	for id := 0; id < num; id++ {
		go busTestReceiver(t, bts[id], pkts, num)
		go busTestSender(t, bts[id], pkts)
	}

	tmout := time.After(30 * time.Second)

	for id := 0; id < num; id++ {
		select {
		case <-bts[id].sdoneq:
			continue
		case <-tmout:
			t.Errorf("Timeout waiting for sender id %d", id)
			return
		}
	}

	for id := 0; id < num; id++ {
		select {
		case <-bts[id].rdoneq:
			continue
		case <-tmout:
			t.Errorf("Timeout waiting for receiver id %d", id)
			return
		}
	}
	t.Logf("All pass")
}
