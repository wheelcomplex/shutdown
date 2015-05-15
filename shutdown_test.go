package shutdown

import (
	"fmt"
	"testing"
	"time"
)

func reset() {
	SetTimeout(1 * time.Second)
	sqM.Lock()
	defer sqM.Unlock()
	srM.Lock()
	defer srM.Unlock()
	shutdownRequested = false
}

func startTimer(t *testing.T) chan struct{} {
	finished := make(chan struct{}, 0)
	go func() {
		srM.RLock()
		timeout := time.After(timeout * 4)
		srM.RUnlock()
		select {
		case <-timeout:
			t.Fatal("timeout while running test")
		case <-finished:
			return

		}
	}()
	return finished
}

func TestBasic(t *testing.T) {
	reset()
	defer close(startTimer(t))
	f := First()
	ok := false
	go func() {
		select {
		case n := <-f:
			ok = true
			close(n)
		}
	}()
	Shutdown()
	if !ok {
		t.Fatal("did not get expected shutdown signal")
	}
	if !Started() {
		t.Fatal("shutdown not marked started")
	}

}

func TestCancel(t *testing.T) {
	reset()
	defer close(startTimer(t))
	f := First()
	ok := false
	go func() {
		select {
		case n := <-f:
			ok = true
			close(n)
		}
	}()
	f.Cancel()
	Shutdown()
	if ok {
		t.Fatal("got unexpected shutdown signal")
	}
}

func TestTimeout(t *testing.T) {
	reset()
	SetTimeout(time.Millisecond * 50)
	defer close(startTimer(t))
	f := First()
	go func() {
		select {
		case <-f:
		}
	}()
	tn := time.Now()
	Shutdown()
	dur := time.Now().Sub(tn)
	if dur > time.Second || dur < time.Millisecond*50 {
		t.Fatalf("timeout time was unexpected:%v", time.Now().Sub(tn))
	}
	if !Started() {
		t.Fatal("got unexpected shutdown signal")
	}
}

func TestLock(t *testing.T) {
	reset()
	defer close(startTimer(t))
	f := First()
	ok := false
	go func() {
		select {
		case n := <-f:
			ok = true
			close(n)
		}
	}()
	got := Lock()
	if !got {
		t.Fatal("Unable to aquire lock")
	}
	Unlock()
	for i := 0; i < 10; i++ {
		go func() {
			if Lock() {
				time.Sleep(time.Second)
				Unlock()
			}
		}()
	}
	Shutdown()
	if !ok {
		t.Fatal("shutdown signal not received")
	}
	if !Started() {
		t.Fatal("expected that shutdown had started")
	}
}

func TestLockUnrelease(t *testing.T) {
	reset()
	defer close(startTimer(t))
	SetTimeout(time.Millisecond * 50)
	got := Lock()
	if !got {
		t.Fatal("Unable to aquire lock")
	}
	tn := time.Now()
	Shutdown()
	dur := time.Now().Sub(tn)
	if dur > time.Second || dur < time.Millisecond*50 {
		t.Fatalf("timeout time was unexpected:%v", time.Now().Sub(tn))
	}
	if !Started() {
		t.Fatal("expected that shutdown had started")
	}
	// Unlock to be nice
	Unlock()
}

func TestOrder(t *testing.T) {
	reset()
	defer close(startTimer(t))

	t3 := Third()
	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	t2 := Second()
	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	t1 := First()
	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	var ok1, ok2, ok3 bool
	go func() {
		for {
			select {
			//t1 must be first
			case n := <-t1:
				if ok1 || ok2 || ok3 {
					t.Fatal("unexpected order", ok1, ok2, ok3)
				}
				ok1 = true
				close(n)
			case n := <-t2:
				if !ok1 || ok2 || ok3 {
					t.Fatal("unexpected order", ok1, ok2, ok3)
				}
				ok2 = true
				close(n)
			case n := <-t3:
				if !ok1 || !ok2 || ok3 {
					t.Fatal("unexpected order", ok1, ok2, ok3)
				}
				ok3 = true
				close(n)
				return
			}
		}
	}()
	if ok1 || ok2 || ok3 {
		t.Fatal("shutdown has already happened", ok1, ok2, ok3)
	}

	Shutdown()
	if !ok1 || !ok2 || !ok3 {
		t.Fatal("did not get expected shutdown signal", ok1, ok2, ok3)
	}
}

func TestRecursive(t *testing.T) {
	reset()
	defer close(startTimer(t))

	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	t1 := First()
	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	var ok1, ok2, ok3 bool
	go func() {
		for {
			select {
			case n := <-t1:
				ok1 = true
				t2 := Second()
				close(n)
				select {
				case n := <-t2:
					ok2 = true
					t3 := Third()
					close(n)
					select {
					case n := <-t3:
						ok3 = true
						close(n)
						return
					}
				}
			}
		}
	}()
	if ok1 || ok2 || ok3 {
		t.Fatal("shutdown has already happened", ok1, ok2, ok3)
	}

	Shutdown()
	if !ok1 || !ok2 || !ok3 {
		t.Fatal("did not get expected shutdown signal", ok1, ok2, ok3)
	}
}

func TestBasicFn(t *testing.T) {
	reset()
	defer close(startTimer(t))
	gotcall := false

	// Register a function
	_ = FirstFunc(func(i interface{}) {
		gotcall = i.(bool)
	}, true)

	// Start shutdown
	Shutdown()
	if !gotcall {
		t.Fatal("did not get expected shutdown signal")
	}
}

func setBool(i interface{}) {
	set := i.(*bool)
	*set = true
}

func TestFnOrder(t *testing.T) {
	reset()
	defer close(startTimer(t))

	var ok1, ok2, ok3 bool
	_ = ThirdFunc(setBool, &ok3)
	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	_ = SecondFunc(setBool, &ok2)
	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	_ = FirstFunc(setBool, &ok1)
	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	if ok1 || ok2 || ok3 {
		t.Fatal("shutdown has already happened", ok1, ok2, ok3)
	}

	Shutdown()

	if !ok1 || !ok2 || !ok3 {
		t.Fatal("did not get expected shutdown signal", ok1, ok2, ok3)
	}
}

func TestFnRecursive(t *testing.T) {
	reset()
	defer close(startTimer(t))

	var ok1, ok2, ok3 bool

	_ = FirstFunc(func(i interface{}) {
		set := i.(*bool)
		*set = true
		_ = SecondFunc(func(i interface{}) {
			set := i.(*bool)
			*set = true
			_ = ThirdFunc(func(i interface{}) {
				set := i.(*bool)
				*set = true
			}, &ok3)
		}, &ok2)
	}, &ok1)

	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	if ok1 || ok2 || ok3 {
		t.Fatal("shutdown has already happened", ok1, ok2, ok3)
	}

	Shutdown()

	if !ok1 || !ok2 || !ok3 {
		t.Fatal("did not get expected shutdown signal", ok1, ok2, ok3)
	}
}

// When setting First or Second inside stage three they should be ignored.
func TestFnRecursiveRev(t *testing.T) {
	reset()
	defer close(startTimer(t))

	var ok1, ok2, ok3 bool

	_ = ThirdFunc(func(i interface{}) {
		set := i.(*bool)
		*set = true
		_ = SecondFunc(func(i interface{}) {
			set := i.(*bool)
			*set = true
		}, &ok2)
		_ = FirstFunc(func(i interface{}) {
			set := i.(*bool)
			*set = true
		}, &ok1)
	}, &ok3)

	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	if ok1 || ok2 || ok3 {
		t.Fatal("shutdown has already happened", ok1, ok2, ok3)
	}

	Shutdown()

	if ok1 || ok2 || !ok3 {
		t.Fatal("did not get expected shutdown signal", ok1, ok2, ok3)
	}
}

func TestFnCancel(t *testing.T) {
	reset()
	defer close(startTimer(t))
	gotcall := false

	// Register a function
	not := FirstFunc(func(i interface{}) {
		gotcall = i.(bool)
	}, true)

	not.Cancel()

	// Start shutdown
	Shutdown()
	if gotcall {
		t.Fatal("got unexpected shutdown signal")
	}
}

func TestFnPanic(t *testing.T) {
	reset()
	defer close(startTimer(t))
	gotcall := false

	// Register a function
	_ = FirstFunc(func(i interface{}) {
		gotcall = i.(bool)
		panic("This is expected")
	}, true)

	// Start shutdown
	Shutdown()
	if !gotcall {
		t.Fatal("did not get expected shutdown signal")
	}
}

func TestFnNotify(t *testing.T) {
	reset()
	defer close(startTimer(t))
	gotcall := false

	// Register a function
	fn := FirstFunc(func(i interface{}) {
		gotcall = i.(bool)
	}, true)

	// Start shutdown
	Shutdown()

	// This must have a notification
	_, ok := <-fn
	if !ok {
		t.Fatal("Notifier was closed before a notification")
	}
	// After this the channel must be closed
	_, ok = <-fn
	if ok {
		t.Fatal("Notifier was not closed after initial notification")
	}
	if !gotcall {
		t.Fatal("did not get expected shutdown signal")
	}
}

func TestFnSingleCancel(t *testing.T) {
	reset()
	defer close(startTimer(t))

	var ok1, ok2, ok3, okcancel bool
	_ = ThirdFunc(func(i interface{}) {
		set := i.(*bool)
		*set = true
	}, &ok3)
	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	_ = SecondFunc(func(i interface{}) {
		set := i.(*bool)
		*set = true
	}, &ok2)
	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	cancel := SecondFunc(func(i interface{}) {
		set := i.(*bool)
		*set = true
	}, &okcancel)
	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	_ = FirstFunc(func(i interface{}) {
		set := i.(*bool)
		*set = true
	}, &ok1)
	if Started() {
		t.Fatal("shutdown started unexpectedly")
	}

	if ok1 || ok2 || ok3 || okcancel {
		t.Fatal("shutdown has already happened", ok1, ok2, ok3, okcancel)
	}

	cancel.Cancel()

	Shutdown()

	if !ok1 || !ok2 || !ok3 || okcancel {
		t.Fatal("did not get expected shutdown signal", ok1, ok2, ok3, okcancel)
	}
}

// Get a notifier and perform our own code when we shutdown
func ExampleNotifier() {
	shutdown := First()
	select {
	case n := <-shutdown:
		// Do shutdown code ...

		// Signal we are done
		close(n)
	}
}

// Get a notifier and perform our own function when we shutdown
func ExampleShutdownFn() {
	_ = FirstFunc(func(i interface{}) {
		// This function is called on shutdown
		fmt.Println(i.(string))
	}, "Example parameter")

	// Will print the parameter when Shutdown() is called

}