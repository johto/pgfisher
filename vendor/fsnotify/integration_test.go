// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !plan9,!solaris

package fsnotify

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// An atomic counter
type counter struct {
	val int32
}

func (c *counter) increment() {
	atomic.AddInt32(&c.val, 1)
}

func (c *counter) value() int32 {
	return atomic.LoadInt32(&c.val)
}

func (c *counter) reset() {
	atomic.StoreInt32(&c.val, 0)
}

// tempMkdir makes a temporary directory
func tempMkdir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "fsnotify")
	if err != nil {
		t.Fatalf("failed to create test directory: %s", err)
	}
	return dir
}

// newWatcher initializes an fsnotify Watcher instance.
func newWatcher(t *testing.T) *Watcher {
	watcher, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher() failed: %s", err)
	}
	return watcher
}

// addWatch adds a watch for a directory
func addWatch(t *testing.T, watcher *Watcher, dir string) {
	if err := watcher.Add(dir); err != nil {
		t.Fatalf("watcher.Add(%q) failed: %s", dir, err)
	}
}

func TestFsnotifyMultipleCreates(t *testing.T) {
	watcher := newWatcher(t)

	// Receive errors on the error channel on a separate goroutine
	go func() {
		for err := range watcher.Errors {
			t.Fatalf("error received: %s", err)
		}
	}()

	// Create directory to watch
	testDir := tempMkdir(t)
	defer os.RemoveAll(testDir)

	testFile := filepath.Join(testDir, "TestFsnotifySeq.testfile")

	addWatch(t, watcher, testDir)

	// Receive events on the event channel on a separate goroutine
	eventstream := watcher.Events
	var createReceived counter
	done := make(chan bool)
	go func() {
		for event := range eventstream {
			// Only count relevant events
			if event.Name == filepath.Clean(testDir) || event.Name == filepath.Clean(testFile) {
				t.Logf("event received: %s", event)
				if event.Op&Create == Create {
					createReceived.increment()
				}
			} else {
				t.Logf("unexpected event received: %s", event)
			}
		}
		done <- true
	}()

	// Create a file
	// This should add at least one event to the fsnotify event queue
	var f *os.File
	f, err := os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("creating test file failed: %s", err)
	}
	f.Sync()

	time.Sleep(time.Millisecond)
	f.WriteString("data")
	f.Sync()
	f.Close()

	time.Sleep(50 * time.Millisecond) // give system time to sync write change before delete

	os.Remove(testFile)

	time.Sleep(50 * time.Millisecond) // give system time to sync write change before delete

	// Recreate the file
	f, err = os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("creating test file failed: %s", err)
	}
	f.Close()
	time.Sleep(50 * time.Millisecond) // give system time to sync write change before delete

	// Modify
	f, err = os.OpenFile(testFile, os.O_WRONLY, 0666)
	if err != nil {
		t.Fatalf("creating test file failed: %s", err)
	}
	f.Sync()

	time.Sleep(time.Millisecond)
	f.WriteString("data")
	f.Sync()
	f.Close()

	time.Sleep(50 * time.Millisecond) // give system time to sync write change before delete

	// Modify
	f, err = os.OpenFile(testFile, os.O_WRONLY, 0666)
	if err != nil {
		t.Fatalf("creating test file failed: %s", err)
	}
	f.Sync()

	time.Sleep(time.Millisecond)
	f.WriteString("data")
	f.Sync()
	f.Close()

	time.Sleep(50 * time.Millisecond) // give system time to sync write change before delete

	// We expect this event to be received almost immediately, but let's wait 500 ms to be sure
	time.Sleep(500 * time.Millisecond)
	cReceived := createReceived.value()
	if cReceived != 2 {
		t.Fatalf("incorrect number of create events received after 500 ms (%d vs %d)", cReceived, 2)
	}

	// Try closing the fsnotify instance
	t.Log("calling Close()")
	watcher.Close()
	t.Log("waiting for the event channel to become closed...")
	select {
	case <-done:
		t.Log("event channel closed")
	case <-time.After(2 * time.Second):
		t.Fatal("event stream was not closed after 2 seconds")
	}
}

func TestFsnotifyDirOnly(t *testing.T) {
	watcher := newWatcher(t)

	// Create directory to watch
	testDir := tempMkdir(t)
	defer os.RemoveAll(testDir)

	// Create a file before watching directory
	// This should NOT add any events to the fsnotify event queue
	testFileAlreadyExists := filepath.Join(testDir, "TestFsnotifyEventsExisting.testfile")
	{
		var f *os.File
		f, err := os.OpenFile(testFileAlreadyExists, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			t.Fatalf("creating test file failed: %s", err)
		}
		f.Sync()
		f.Close()
	}

	addWatch(t, watcher, testDir)

	// Receive errors on the error channel on a separate goroutine
	go func() {
		for err := range watcher.Errors {
			t.Fatalf("error received: %s", err)
		}
	}()

	testFile := filepath.Join(testDir, "TestFsnotifyDirOnly.testfile")

	// Receive events on the event channel on a separate goroutine
	eventstream := watcher.Events
	var createReceived counter
	done := make(chan bool)
	go func() {
		for event := range eventstream {
			// Only count relevant events
			if event.Name == filepath.Clean(testDir) || event.Name == filepath.Clean(testFile) || event.Name == filepath.Clean(testFileAlreadyExists) {
				t.Logf("event received: %s", event)
				if event.Op&Create == Create {
					createReceived.increment()
				}
			} else {
				t.Logf("unexpected event received: %s", event)
			}
		}
		done <- true
	}()

	// Create a file
	// This should add at least one event to the fsnotify event queue
	var f *os.File
	f, err := os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("creating test file failed: %s", err)
	}
	f.Sync()

	time.Sleep(time.Millisecond)
	f.WriteString("data")
	f.Sync()
	f.Close()

	time.Sleep(50 * time.Millisecond) // give system time to sync write change before delete

	os.Remove(testFile)
	os.Remove(testFileAlreadyExists)

	// We expect this event to be received almost immediately, but let's wait 500 ms to be sure
	time.Sleep(500 * time.Millisecond)
	cReceived := createReceived.value()
	if cReceived != 1 {
		t.Fatalf("incorrect number of create events received after 500 ms (%d vs %d)", cReceived, 1)
	}

	// Try closing the fsnotify instance
	t.Log("calling Close()")
	watcher.Close()
	t.Log("waiting for the event channel to become closed...")
	select {
	case <-done:
		t.Log("event channel closed")
	case <-time.After(2 * time.Second):
		t.Fatal("event stream was not closed after 2 seconds")
	}
}

func TestFsnotifySubDir(t *testing.T) {
	watcher := newWatcher(t)

	// Create directory to watch
	testDir := tempMkdir(t)
	defer os.RemoveAll(testDir)

	testFile1 := filepath.Join(testDir, "TestFsnotifyFile1.testfile")
	testSubDir := filepath.Join(testDir, "sub")
	testSubDirFile := filepath.Join(testDir, "sub/TestFsnotifyFile1.testfile")

	// Receive errors on the error channel on a separate goroutine
	go func() {
		for err := range watcher.Errors {
			t.Fatalf("error received: %s", err)
		}
	}()

	// Receive events on the event channel on a separate goroutine
	eventstream := watcher.Events
	var createReceived counter
	done := make(chan bool)
	go func() {
		for event := range eventstream {
			// Only count relevant events
			if event.Name == filepath.Clean(testDir) || event.Name == filepath.Clean(testSubDir) || event.Name == filepath.Clean(testFile1) {
				t.Logf("event received: %s", event)
				if event.Op&Create == Create {
					createReceived.increment()
				}
			} else {
				t.Logf("unexpected event received: %s", event)
			}
		}
		done <- true
	}()

	addWatch(t, watcher, testDir)

	// Create sub-directory
	if err := os.Mkdir(testSubDir, 0777); err != nil {
		t.Fatalf("failed to create test sub-directory: %s", err)
	}

	// Create a file
	var f *os.File
	f, err := os.OpenFile(testFile1, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("creating test file failed: %s", err)
	}
	f.Sync()
	f.Close()

	// Create a file (Should not see this! we are not watching subdir)
	var fs *os.File
	fs, err = os.OpenFile(testSubDirFile, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("creating test file failed: %s", err)
	}
	fs.Sync()
	fs.Close()

	time.Sleep(200 * time.Millisecond)

	// Make sure receive deletes for both file and sub-directory
	os.RemoveAll(testSubDir)
	os.Remove(testFile1)

	// We expect this event to be received almost immediately, but let's wait 500 ms to be sure
	time.Sleep(500 * time.Millisecond)
	cReceived := createReceived.value()
	if cReceived != 2 {
		t.Fatalf("incorrect number of create events received after 500 ms (%d vs %d)", cReceived, 2)
	}

	// Try closing the fsnotify instance
	t.Log("calling Close()")
	watcher.Close()
	t.Log("waiting for the event channel to become closed...")
	select {
	case <-done:
		t.Log("event channel closed")
	case <-time.After(2 * time.Second):
		t.Fatal("event stream was not closed after 2 seconds")
	}
}

func TestFsnotifyRenameToCreate(t *testing.T) {
	watcher := newWatcher(t)

	// Create directory to watch
	testDir := tempMkdir(t)
	defer os.RemoveAll(testDir)

	// Create directory to get file
	testDirFrom := tempMkdir(t)
	defer os.RemoveAll(testDirFrom)

	addWatch(t, watcher, testDir)

	// Receive errors on the error channel on a separate goroutine
	go func() {
		for err := range watcher.Errors {
			t.Fatalf("error received: %s", err)
		}
	}()

	testFile := filepath.Join(testDirFrom, "TestFsnotifyEvents.testfile")
	testFileRenamed := filepath.Join(testDir, "TestFsnotifyEvents.testfileRenamed")

	// Receive events on the event channel on a separate goroutine
	eventstream := watcher.Events
	var createReceived counter
	done := make(chan bool)
	go func() {
		for event := range eventstream {
			// Only count relevant events
			if event.Name == filepath.Clean(testDir) || event.Name == filepath.Clean(testFile) || event.Name == filepath.Clean(testFileRenamed) {
				if event.Op&Create == Create {
					createReceived.increment()
				}
				t.Logf("event received: %s", event)
			} else {
				t.Logf("unexpected event received: %s", event)
			}
		}
		done <- true
	}()

	// Create a file
	// This should add at least one event to the fsnotify event queue
	var f *os.File
	f, err := os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("creating test file failed: %s", err)
	}
	f.Sync()
	f.Close()

	if err := testRename(testFile, testFileRenamed); err != nil {
		t.Fatalf("rename failed: %s", err)
	}

	// We expect this event to be received almost immediately, but let's wait 500 ms to be sure
	time.Sleep(500 * time.Millisecond)
	if createReceived.value() == 0 {
		t.Fatal("fsnotify create events have not been received after 500 ms")
	}

	// Try closing the fsnotify instance
	t.Log("calling Close()")
	watcher.Close()
	t.Log("waiting for the event channel to become closed...")
	select {
	case <-done:
		t.Log("event channel closed")
	case <-time.After(2 * time.Second):
		t.Fatal("event stream was not closed after 2 seconds")
	}

	os.Remove(testFileRenamed)
}

func TestFsnotifyRenameToOverwrite(t *testing.T) {
	switch runtime.GOOS {
	case "plan9", "windows":
		t.Skipf("skipping test on %q (os.Rename over existing file does not create event).", runtime.GOOS)
	}

	watcher := newWatcher(t)

	// Create directory to watch
	testDir := tempMkdir(t)
	defer os.RemoveAll(testDir)

	// Create directory to get file
	testDirFrom := tempMkdir(t)
	defer os.RemoveAll(testDirFrom)

	testFile := filepath.Join(testDirFrom, "TestFsnotifyEvents.testfile")
	testFileRenamed := filepath.Join(testDir, "TestFsnotifyEvents.testfileRenamed")

	// Create a file
	var fr *os.File
	fr, err := os.OpenFile(testFileRenamed, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("creating test file failed: %s", err)
	}
	fr.Sync()
	fr.Close()

	addWatch(t, watcher, testDir)

	// Receive errors on the error channel on a separate goroutine
	go func() {
		for err := range watcher.Errors {
			t.Fatalf("error received: %s", err)
		}
	}()

	// Receive events on the event channel on a separate goroutine
	eventstream := watcher.Events
	var eventReceived counter
	done := make(chan bool)
	go func() {
		for event := range eventstream {
			// Only count relevant events
			if event.Name == filepath.Clean(testFileRenamed) {
				eventReceived.increment()
				t.Logf("event received: %s", event)
			} else {
				t.Logf("unexpected event received: %s", event)
			}
		}
		done <- true
	}()

	// Create a file
	// This should add at least one event to the fsnotify event queue
	var f *os.File
	f, err = os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("creating test file failed: %s", err)
	}
	f.Sync()
	f.Close()

	if err := testRename(testFile, testFileRenamed); err != nil {
		t.Fatalf("rename failed: %s", err)
	}

	// We expect this event to be received almost immediately, but let's wait 500 ms to be sure
	time.Sleep(500 * time.Millisecond)
	if eventReceived.value() == 0 {
		t.Fatal("fsnotify events have not been received after 500 ms")
	}

	// Try closing the fsnotify instance
	t.Log("calling Close()")
	watcher.Close()
	t.Log("waiting for the event channel to become closed...")
	select {
	case <-done:
		t.Log("event channel closed")
	case <-time.After(2 * time.Second):
		t.Fatal("event stream was not closed after 2 seconds")
	}

	os.Remove(testFileRenamed)
}

func TestRemovalOfWatch(t *testing.T) {
	// Create directory to watch
	testDir := tempMkdir(t)
	defer os.RemoveAll(testDir)

	// Create a file before watching directory
	testFileAlreadyExists := filepath.Join(testDir, "TestFsnotifyEventsExisting.testfile")
	{
		var f *os.File
		f, err := os.OpenFile(testFileAlreadyExists, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			t.Fatalf("creating test file failed: %s", err)
		}
		f.Sync()
		f.Close()
	}

	watcher := newWatcher(t)
	defer watcher.Close()

	addWatch(t, watcher, testDir)
	if err := watcher.Remove(testDir); err != nil {
		t.Fatalf("Could not remove the watch: %v\n", err)
	}

	go func() {
		select {
		case ev := <-watcher.Events:
			t.Fatalf("We received event: %v\n", ev)
		case <-time.After(500 * time.Millisecond):
			t.Log("No event received, as expected.")
		}
	}()

	time.Sleep(200 * time.Millisecond)
	// Modify the file outside of the watched dir
	f, err := os.Open(testFileAlreadyExists)
	if err != nil {
		t.Fatalf("Open test file failed: %s", err)
	}
	f.WriteString("data")
	f.Sync()
	f.Close()
	if err := os.Chmod(testFileAlreadyExists, 0700); err != nil {
		t.Fatalf("chmod failed: %s", err)
	}
	time.Sleep(400 * time.Millisecond)
}

func TestFsnotifyClose(t *testing.T) {
	watcher := newWatcher(t)
	watcher.Close()

	var done int32
	go func() {
		watcher.Close()
		atomic.StoreInt32(&done, 1)
	}()

	time.Sleep(50e6) // 50 ms
	if atomic.LoadInt32(&done) == 0 {
		t.Fatal("double Close() test failed: second Close() call didn't return")
	}

	testDir := tempMkdir(t)
	defer os.RemoveAll(testDir)

	if err := watcher.Add(testDir); err == nil {
		t.Fatal("expected error on Watch() after Close(), got nil")
	}
}

func TestFsnotifyFakeSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks don't work on Windows.")
	}

	watcher := newWatcher(t)

	// Create directory to watch
	testDir := tempMkdir(t)
	defer os.RemoveAll(testDir)

	var errorsReceived counter
	// Receive errors on the error channel on a separate goroutine
	go func() {
		for errors := range watcher.Errors {
			t.Logf("Received error: %s", errors)
			errorsReceived.increment()
		}
	}()

	// Count the CREATE events received
	var createEventsReceived, otherEventsReceived counter
	go func() {
		for ev := range watcher.Events {
			t.Logf("event received: %s", ev)
			if ev.Op&Create == Create {
				createEventsReceived.increment()
			} else {
				otherEventsReceived.increment()
			}
		}
	}()

	addWatch(t, watcher, testDir)

	if err := os.Symlink(filepath.Join(testDir, "zzz"), filepath.Join(testDir, "zzznew")); err != nil {
		t.Fatalf("Failed to create bogus symlink: %s", err)
	}
	t.Logf("Created bogus symlink")

	// We expect this event to be received almost immediately, but let's wait 500 ms to be sure
	time.Sleep(500 * time.Millisecond)

	// Should not be error, just no events for broken links (watching nothing)
	if errorsReceived.value() > 0 {
		t.Fatal("fsnotify errors have been received.")
	}
	if otherEventsReceived.value() > 0 {
		t.Fatal("fsnotify other events received on the broken link")
	}

	// Except for 1 create event (for the link itself)
	if createEventsReceived.value() == 0 {
		t.Fatal("fsnotify create events were not received after 500 ms")
	}
	if createEventsReceived.value() > 1 {
		t.Fatal("fsnotify more create events received than expected")
	}

	// Try closing the fsnotify instance
	t.Log("calling Close()")
	watcher.Close()
}

// TestConcurrentRemovalOfWatch tests that concurrent calls to RemoveWatch do not race.
// See https://codereview.appspot.com/103300045/
// go test -test.run=TestConcurrentRemovalOfWatch -test.cpu=1,1,1,1,1 -race
func TestConcurrentRemovalOfWatch(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("regression test for race only present on darwin")
	}

	// Create directory to watch
	testDir := tempMkdir(t)
	defer os.RemoveAll(testDir)

	// Create a file before watching directory
	testFileAlreadyExists := filepath.Join(testDir, "TestFsnotifyEventsExisting.testfile")
	{
		var f *os.File
		f, err := os.OpenFile(testFileAlreadyExists, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			t.Fatalf("creating test file failed: %s", err)
		}
		f.Sync()
		f.Close()
	}

	watcher := newWatcher(t)
	defer watcher.Close()

	addWatch(t, watcher, testDir)

	// Test that RemoveWatch can be invoked concurrently, with no data races.
	removed1 := make(chan struct{})
	go func() {
		defer close(removed1)
		watcher.Remove(testDir)
	}()
	removed2 := make(chan struct{})
	go func() {
		close(removed2)
		watcher.Remove(testDir)
	}()
	<-removed1
	<-removed2
}

func TestClose(t *testing.T) {
	// Regression test for #59 bad file descriptor from Close
	testDir := tempMkdir(t)
	defer os.RemoveAll(testDir)

	watcher := newWatcher(t)
	if err := watcher.Add(testDir); err != nil {
		t.Fatalf("Expected no error on Add, got %v", err)
	}
	err := watcher.Close()
	if err != nil {
		t.Fatalf("Expected no error on Close, got %v.", err)
	}
}

func testRename(file1, file2 string) error {
	switch runtime.GOOS {
	case "windows", "plan9":
		return os.Rename(file1, file2)
	default:
		cmd := exec.Command("mv", file1, file2)
		return cmd.Run()
	}
}
