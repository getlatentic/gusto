package node

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
)

// A node that starts tells gusto cloud so at once, and gusto cloud places on it and reaches it even while its own view of the
// shared directory does not show the node yet.
func TestANodeThatStartsTellsGustoCloudWhichPlacesOnItAtOnce(t *testing.T) {
	pool := &Pool{Dir: filepath.Join(t.TempDir(), "seen-by-gusto-cloud"), Secret: secret, MaxAge: time.Minute, MinFreeMiB: 1024}
	var woken atomic.Int32
	front := httptest.NewServer(pool.Started(func() { woken.Add(1) }))
	t.Cleanup(front.Close)

	nodes := filepath.Join(t.TempDir(), "written-by-the-node")
	if err := TellStartedAt(nodes, StartedAddress(front.URL+"/")); err != nil {
		t.Fatal(err)
	}
	agent := &Agent{Name: "cell-0abc", Private: "127.0.0.1", Secret: secret, Servers: &cloud.LocalNode{},
		Measure: func() Capacity { return Capacity{CPUs: 2, MemoryAvailableMiB: 8000} }}
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan error, 1)
	go func() { ran <- agent.Run(ctx, "127.0.0.1:0", nodes, time.Hour) }()
	t.Cleanup(func() { cancel(); <-ran })

	waitFor(t, "gusto cloud was told the node started", func() bool { return woken.Load() == 1 })
	placed, err := pool.Place()
	if err != nil || placed.Name() != agent.Name {
		t.Fatalf("the node that said it started was not placed on: %v %v", placed, err)
	}
	if _, err := pool.Get(agent.Name); err != nil {
		t.Errorf("the node that said it started cannot be reached: %v", err)
	}
}

// Only a node holding the secret is heard, and only with a registration.
func TestAWordThatANodeStartedIsHeardOnlyWhenSignedAndWhole(t *testing.T) {
	pool := &Pool{Dir: filepath.Join(t.TempDir(), "cells"), Secret: secret, MaxAge: time.Minute}
	var woken atomic.Int32
	handler := pool.Started(func() { woken.Add(1) })
	send := func(key []byte, registration any) int {
		body, _ := json.Marshal(registration)
		request := httptest.NewRequest(http.MethodPost, "/nodes/started", bytes.NewReader(body))
		if key != nil {
			sign(request, key, time.Now(), body)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Code
	}
	whole := Registration{Name: "cell-0abc", Agent: "127.0.0.1:1", Private: "127.0.0.1", Beat: time.Now().Unix()}
	if code := send(nil, whole); code != http.StatusUnauthorized {
		t.Errorf("an unsigned word was answered %d", code)
	}
	if code := send([]byte("another secret, as long as the real one"), whole); code != http.StatusUnauthorized {
		t.Errorf("a word signed with another secret was answered %d", code)
	}
	if code := send(secret, Registration{Name: "Not A Name", Agent: "127.0.0.1:1", Private: "127.0.0.1"}); code != http.StatusBadRequest {
		t.Errorf("a word with no registration in it was answered %d", code)
	}
	if woken.Load() != 0 {
		t.Fatal("a word that was refused woke gusto cloud")
	}
	if fresh, _ := pool.fresh(); len(fresh) != 0 {
		t.Fatalf("a word that was refused was held: %v", fresh)
	}
	if code := send(secret, whole); code != http.StatusNoContent || woken.Load() != 1 {
		t.Errorf("a signed word was answered %d and woke gusto cloud %d times", code, woken.Load())
	}
}

// What a node said when it started is held until it is as old as a silent node may be, after which only its file counts.
func TestWhatANodeSaidWhenItStartedIsHeldOnlyWhileItIsFresh(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	pool := &Pool{Dir: filepath.Join(t.TempDir(), "cells"), Secret: secret, MaxAge: 30 * time.Second, now: func() time.Time { return now }}
	pool.hold(Registration{Name: "cell-0abc", Agent: "127.0.0.1:1", Private: "127.0.0.1", Beat: now.Unix()})
	if fresh, _ := pool.fresh(); len(fresh) != 1 {
		t.Fatalf("a node that just said it started is not fresh: %v", fresh)
	}
	if _, age, err := pool.registered("cell-0abc"); err != nil || age != 0 {
		t.Errorf("a node that just said it started is not registered: %s %v", age, err)
	}
	now = now.Add(31 * time.Second)
	if fresh, _ := pool.fresh(); len(fresh) != 0 {
		t.Errorf("a node silent since it started is still fresh: %v", fresh)
	}
}
