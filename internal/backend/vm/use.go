package vm

import "sync"

// computerUse counts the tasks working in each computer, which keeps it from being stopped as idle. A task is counted before
// its sandbox starts, and the idle stop holds the computer's lock from its look to the stop, so a task cannot start in
// between and be stopped. It counts tasks by the computer they run in, whoever asked them. spec/Computer.tla is the model.
type computerUse struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	tasks map[string]int
}

func newComputerUse() *computerUse {
	return &computerUse{locks: map[string]*sync.Mutex{}, tasks: map[string]int{}}
}

// lockOf is a computer's lock. There is one for each computer of the organization's people, and no more.
func (u *computerUse) lockOf(id string) *sync.Mutex {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.locks[id] == nil {
		u.locks[id] = &sync.Mutex{}
	}
	return u.locks[id]
}

// enter counts a task in a computer, after any stop under way has finished.
func (u *computerUse) enter(id string) {
	held := u.lockOf(id)
	held.Lock()
	defer held.Unlock()
	u.mu.Lock()
	defer u.mu.Unlock()
	u.tasks[id]++
}

// leave counts a task out of a computer.
func (u *computerUse) leave(id string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.tasks[id]--; u.tasks[id] <= 0 {
		delete(u.tasks, id)
	}
}

// stopIdle runs stop when no task is in the computer, holding its lock so none starts meanwhile, and says whether it did. The
// lock is held across stop, which is a call to the gateway: what waits on it is only a task about to start in this computer,
// which has to wait for the stop anyway.
func (u *computerUse) stopIdle(id string, stop func()) bool {
	held := u.lockOf(id)
	held.Lock()
	defer held.Unlock()
	u.mu.Lock()
	busy := u.tasks[id] > 0
	u.mu.Unlock()
	if busy {
		return false
	}
	stop()
	return true
}
