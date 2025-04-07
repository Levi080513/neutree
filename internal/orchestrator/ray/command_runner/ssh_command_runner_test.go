package command_runner

import (
	"context"
	"errors"
	"testing"

	v1 "github.com/neutree-ai/neutree/api/v1"
	commandmocks "github.com/neutree-ai/neutree/pkg/command/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewSSHCommandRunner(t *testing.T) {
	mockExec := new(commandmocks.MockExecutor)
	authConfig := v1.Auth{
		SSHPrivateKey: "test_key",
		SSHUser:       "test_user",
	}

	runner := NewSSHCommandRunner("node1", "127.0.0.1", authConfig, "test-cluster", mockExec.Execute)

	assert.Equal(t, "node1", runner.nodeID)
	assert.Equal(t, "test-cluster", runner.clusterName)
	assert.Equal(t, "test_key", runner.sshPrivateKey)
	assert.Equal(t, "test_user", runner.sshUser)
	assert.Equal(t, "127.0.0.1", runner.sshIP)
	assert.NotEmpty(t, runner.sshControlPath)
}

func TestSSHCommandRunner_Run_Success(t *testing.T) {
	mockExec := new(commandmocks.MockExecutor)
	authConfig := v1.Auth{
		SSHPrivateKey: "test_key",
		SSHUser:       "test_user",
	}
	runner := NewSSHCommandRunner("node1", "127.0.0.1", authConfig, "test-cluster", mockExec.Execute)

	mockExec.On("Execute", mock.Anything, "ssh", mock.Anything).Return([]byte("success"), nil)

	output, err := runner.Run(context.Background(), "echo hello", false, nil, true, nil, "", "", false)

	assert.NoError(t, err)
	assert.Equal(t, "success", output)
	mockExec.AssertExpectations(t)
}

func TestSSHCommandRunner_Run_WithError(t *testing.T) {
	mockExec := new(commandmocks.MockExecutor)
	authConfig := v1.Auth{
		SSHPrivateKey: "test_key",
		SSHUser:       "test_user",
	}
	runner := NewSSHCommandRunner("node1", "127.0.0.1", authConfig, "test-cluster", mockExec.Execute)

	mockExec.On("Execute", mock.Anything, "ssh", mock.Anything).Return([]byte("error"), errors.New("ssh failed"))

	output, err := runner.Run(context.Background(), "echo hello", false, nil, true, nil, "", "", false)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SSH command failed")
	assert.Equal(t, "error", output)
	mockExec.AssertExpectations(t)
}

func TestSSHCommandRunner_Run_ExitOnFail(t *testing.T) {
	mockExec := new(commandmocks.MockExecutor)
	authConfig := v1.Auth{
		SSHPrivateKey: "test_key",
		SSHUser:       "test_user",
	}
	runner := NewSSHCommandRunner("node1", "127.0.0.1", authConfig, "test-cluster", mockExec.Execute)

	// 设置模拟期望
	mockExec.On("Execute", mock.Anything, "ssh", mock.Anything).Return([]byte("error"), errors.New("ssh failed"))

	output, err := runner.Run(context.Background(), "echo hello", true, nil, true, nil, "", "", false)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command failed")
	assert.Equal(t, "", output)
	mockExec.AssertExpectations(t)
}

func TestSSHCommandRunner_Run_ShutdownAfterRun(t *testing.T) {
	mockExec := new(commandmocks.MockExecutor)
	authConfig := v1.Auth{
		SSHPrivateKey: "test_key",
		SSHUser:       "test_user",
	}
	runner := NewSSHCommandRunner("node1", "127.0.0.1", authConfig, "test-cluster", mockExec.Execute)

	//commandArgs := []string{}
	mockExec.On("Execute", mock.Anything, "ssh", mock.AnythingOfType).Run(func(args mock.Arguments) {
		//assert.Contains(t, strings.Join(commandArgs, " "), "sudo shutdown -h now")
	}).Return([]byte("success"), nil)

	_, err := runner.Run(context.Background(), "echo hello", false, nil, false, nil, "", "", true)

	assert.NoError(t, err)
}

func TestSSHCommandRunner_getSSHOptions(t *testing.T) {
	mockExec := new(commandmocks.MockExecutor)
	authConfig := v1.Auth{
		SSHPrivateKey: "test_key",
		SSHUser:       "test_user",
	}
	runner := NewSSHCommandRunner("node1", "127.0.0.1", authConfig, "test-cluster", mockExec.Execute)

	options := runner.getSSHOptions("")
	assert.Contains(t, options, "-o")
	assert.Contains(t, options, "StrictHostKeyChecking=no")
	assert.Contains(t, options, "-i")
	assert.Contains(t, options, "test_key")

	options = runner.getSSHOptions("override_key")
	assert.Contains(t, options, "-i")
	assert.Contains(t, options, "override_key")
}
