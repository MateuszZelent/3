package main

import "testing"

func TestQueueStateRecordsPIDExitAndTunnelError(t *testing.T) {
	s := &stateTab{jobs: []job{{uid: 0, inFile: "job.mx3", state: JobQueued, gpu: -1}}, tunnelRequired: true}
	j, ok := s.StartNext("127.0.0.1:35368")
	if !ok {
		t.Fatal("job did not start")
	}
	s.SetGPU(j.uid, 2)
	s.SetPID(j.uid, 1234)
	if s.jobs[0].state != JobRunning || s.jobs[0].pid != 1234 {
		t.Fatalf("running state = %+v", s.jobs[0])
	}
	s.SetReady(j.uid, "127.0.0.1:35369")
	s.SetExitCode(j.uid, 0)
	if s.jobs[0].state != JobReady || !s.jobs[0].exitCodeSet || s.jobs[0].exitCode != 0 {
		t.Fatalf("ready state = %+v", s.jobs[0])
	}
	s.SetTunnelError(j.uid, "known_hosts missing")
	if s.jobs[0].state != JobFailed || s.jobs[0].tunnelError != "known_hosts missing" {
		t.Fatalf("tunnel failure state = %+v", s.jobs[0])
	}
}

func TestParseTunnelFailedMarker(t *testing.T) {
	err, ok := parseTunnelFailed("//tunnel-failed SSH known_hosts unavailable")
	if !ok || err == nil || err.Error() != "SSH known_hosts unavailable" {
		t.Fatalf("error = %v, ok = %v", err, ok)
	}
	if _, ok := parseTunnelFailed("normal output"); ok {
		t.Fatal("normal output parsed as tunnel failure")
	}
}
