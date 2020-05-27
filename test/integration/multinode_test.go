// +build integration

/*
Copyright 2020 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestMultiNode(t *testing.T) {
	if NoneDriver() {
		t.Skip("none driver does not support multinode")
	}

	type validatorFunc func(context.Context, *testing.T, string)
	profile := UniqueProfileName("multinode")
	ctx, cancel := context.WithTimeout(context.Background(), Minutes(30))
	defer CleanupWithLogs(t, profile, cancel)

	t.Run("serial", func(t *testing.T) {
		tests := []struct {
			name      string
			validator validatorFunc
		}{
			{"FreshStart2Nodes", validateMultiNodeStart},
			{"AddNode", validateAddNodeToMultiNode},
			{"StopNode", validateStopRunningNode},
			{"StartAfterStop", validateStartNodeAfterStop},
			{"DeleteNode", validateDeleteNodeFromMultiNode},
			{"StopMultiNode", validateStopMultiNodeCluster},
			{"RestartMultiNode", validateRestartMultiNodeCluster},
		}
		for _, tc := range tests {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				defer PostMortemLogs(t, profile)
				tc.validator(ctx, t, profile)
			})
		}
	})
}

func validateMultiNodeStart(ctx context.Context, t *testing.T, profile string) {
	// Start a 2 node cluster with the --nodes param
	startArgs := append([]string{"start", "-p", profile, "--wait=true", "--memory=2200", "--nodes=2"}, StartArgs()...)
	rr, err := Run(t, exec.CommandContext(ctx, Target(), startArgs...))
	if err != nil {
		t.Fatalf("failed to start cluster. args %q : %v", rr.Command(), err)
	}

	// Make sure minikube status shows 2 nodes
	rr, err = Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "status", "--alsologtostderr"))
	if err != nil {
		t.Fatalf("failed to run minikube status. args %q : %v", rr.Command(), err)
	}

	if strings.Count(rr.Stdout.String(), "host: Running") != 2 {
		t.Errorf("status says both hosts are not running: args %q: %v", rr.Command(), rr.Stdout.String())
	}

	if strings.Count(rr.Stdout.String(), "kubelet: Running") != 2 {
		t.Errorf("status says both kubelets are not running: args %q: %v", rr.Command(), rr.Stdout.String())
	}

}

func validateAddNodeToMultiNode(ctx context.Context, t *testing.T, profile string) {
	// Add a node to the current cluster
	addArgs := []string{"node", "add", "-p", profile, "-v", "3", "--alsologtostderr"}
	rr, err := Run(t, exec.CommandContext(ctx, Target(), addArgs...))
	if err != nil {
		t.Fatalf("failed to add node to current cluster. args %q : %v", rr.Command(), err)
	}

	// Make sure minikube status shows 3 nodes
	rr, err = Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "status", "--alsologtostderr"))
	if err != nil {
		t.Fatalf("failed to run minikube status. args %q : %v", rr.Command(), err)
	}

	if strings.Count(rr.Stdout.String(), "host: Running") != 3 {
		t.Errorf("status says all hosts are not running: args %q: %v", rr.Command(), rr.Stdout.String())
	}

	if strings.Count(rr.Stdout.String(), "kubelet: Running") != 3 {
		t.Errorf("status says all kubelets are not running: args %q: %v", rr.Command(), rr.Stdout.String())
	}
}

func validateStopRunningNode(ctx context.Context, t *testing.T, profile string) {
	// Run minikube node stop on that node
	rr, err := Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "node", "stop", ThirdNodeName))
	if err != nil {
		t.Errorf("node stop returned an error. args %q: %v", rr.Command(), err)
	}

	// Run status again to see the stopped host
	rr, err = Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "status"))
	// Exit code 7 means one host is stopped, which we are expecting
	if err != nil && rr.ExitCode != 7 {
		t.Fatalf("failed to run minikube status. args %q : %v", rr.Command(), err)
	}

	// Make sure minikube status shows 2 running nodes and 1 stopped one
	rr, err = Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "status", "--alsologtostderr"))
	if err != nil && rr.ExitCode != 7 {
		t.Fatalf("failed to run minikube status. args %q : %v", rr.Command(), err)
	}

	if strings.Count(rr.Stdout.String(), "kubelet: Running") != 2 {
		t.Errorf("incorrect number of running kubelets: args %q: %v", rr.Command(), rr.Stdout.String())
	}

	if strings.Count(rr.Stdout.String(), "host: Stopped") != 1 {
		t.Errorf("incorrect number of stopped hosts: args %q: %v", rr.Command(), rr.Stdout.String())
	}

	if strings.Count(rr.Stdout.String(), "kubelet: Stopped") != 1 {
		t.Errorf("incorrect number of stopped kubelets: args %q: %v", rr.Command(), rr.Stdout.String())
	}
}

func validateStartNodeAfterStop(ctx context.Context, t *testing.T, profile string) {
	if DockerDriver() {
		rr, err := Run(t, exec.Command("docker", "version", "-f", "{{.Server.Version}}"))
		if err != nil {
			t.Fatalf("docker is broken: %v", err)
		}
		if strings.Contains(rr.Stdout.String(), "azure") {
			t.Skip("kic containers are not supported on docker's azure")
		}
	}

	// Start the node back up
	rr, err := Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "node", "start", ThirdNodeName, "--alsologtostderr"))
	if err != nil {
		t.Logf(rr.Stderr.String())
		t.Errorf("node start returned an error. args %q: %v", rr.Command(), err)
	}

	// Make sure minikube status shows 3 running hosts
	rr, err = Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "status"))
	if err != nil {
		t.Fatalf("failed to run minikube status. args %q : %v", rr.Command(), err)
	}

	if strings.Count(rr.Stdout.String(), "host: Running") != 3 {
		t.Errorf("status says both hosts are not running: args %q: %v", rr.Command(), rr.Stdout.String())
	}

	if strings.Count(rr.Stdout.String(), "kubelet: Running") != 3 {
		t.Errorf("status says both kubelets are not running: args %q: %v", rr.Command(), rr.Stdout.String())
	}

	// Make sure kubectl can connect correctly
	rr, err = Run(t, exec.CommandContext(ctx, "kubectl", "get", "nodes"))
	if err != nil {
		t.Fatalf("failed to kubectl get nodes. args %q : %v", rr.Command(), err)
	}
}

func validateStopMultiNodeCluster(ctx context.Context, t *testing.T, profile string) {
	// Run minikube node stop on that node
	rr, err := Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "stop"))
	if err != nil {
		t.Errorf("node stop returned an error. args %q: %v", rr.Command(), err)
	}

	// Run status to see the stopped hosts
	rr, err = Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "status"))
	// Exit code 7 means one host is stopped, which we are expecting
	if err != nil && rr.ExitCode != 7 {
		t.Fatalf("failed to run minikube status. args %q : %v", rr.Command(), err)
	}

	// Make sure minikube status shows 2 stopped nodes
	rr, err = Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "status", "--alsologtostderr"))
	if err != nil && rr.ExitCode != 7 {
		t.Fatalf("failed to run minikube status. args %q : %v", rr.Command(), err)
	}

	if strings.Count(rr.Stdout.String(), "host: Stopped") != 2 {
		t.Errorf("incorrect number of stopped hosts: args %q: %v", rr.Command(), rr.Stdout.String())
	}

	if strings.Count(rr.Stdout.String(), "kubelet: Stopped") != 2 {
		t.Errorf("incorrect number of stopped kubelets: args %q: %v", rr.Command(), rr.Stdout.String())
	}
}

func validateRestartMultiNodeCluster(ctx context.Context, t *testing.T, profile string) {
	if DockerDriver() {
		rr, err := Run(t, exec.Command("docker", "version", "-f", "{{.Server.Version}}"))
		if err != nil {
			t.Fatalf("docker is broken: %v", err)
		}
		if strings.Contains(rr.Stdout.String(), "azure") {
			t.Skip("kic containers are not supported on docker's azure")
		}
	}
	// Restart a full cluster with minikube start
	startArgs := append([]string{"start", "-p", profile}, StartArgs()...)
	rr, err := Run(t, exec.CommandContext(ctx, Target(), startArgs...))
	if err != nil {
		t.Fatalf("failed to start cluster. args %q : %v", rr.Command(), err)
	}

	// Make sure minikube status shows 2 running nodes
	rr, err = Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "status", "--alsologtostderr"))
	if err != nil {
		t.Fatalf("failed to run minikube status. args %q : %v", rr.Command(), err)
	}

	if strings.Count(rr.Stdout.String(), "host: Running") != 2 {
		t.Errorf("status says both hosts are not running: args %q: %v", rr.Command(), rr.Stdout.String())
	}

	if strings.Count(rr.Stdout.String(), "kubelet: Running") != 2 {
		t.Errorf("status says both kubelets are not running: args %q: %v", rr.Command(), rr.Stdout.String())
	}
}

func validateDeleteNodeFromMultiNode(ctx context.Context, t *testing.T, profile string) {

	// Start the node back up
	rr, err := Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "node", "delete", ThirdNodeName))
	if err != nil {
		t.Errorf("node stop returned an error. args %q: %v", rr.Command(), err)
	}

	// Make sure status is back down to 2 hosts
	rr, err = Run(t, exec.CommandContext(ctx, Target(), "-p", profile, "status", "--alsologtostderr"))
	if err != nil {
		t.Fatalf("failed to run minikube status. args %q : %v", rr.Command(), err)
	}

	if strings.Count(rr.Stdout.String(), "host: Running") != 2 {
		t.Errorf("status says both hosts are not running: args %q: %v", rr.Command(), rr.Stdout.String())
	}

	if strings.Count(rr.Stdout.String(), "kubelet: Running") != 2 {
		t.Errorf("status says both kubelets are not running: args %q: %v", rr.Command(), rr.Stdout.String())
	}

	if DockerDriver() {
		rr, err := Run(t, exec.Command("docker", "volume", "ls"))
		if err != nil {
			t.Errorf("failed to run %q : %v", rr.Command(), err)
		}
		if strings.Contains(rr.Stdout.String(), fmt.Sprintf("%s-%s", profile, ThirdNodeName)) {
			t.Errorf("docker volume was not properly deleted: %s", rr.Stdout.String())
		}
	}

}
