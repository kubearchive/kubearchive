// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigOptions_newSetupCmd(t *testing.T) {
	opts := NewConfigOptions()
	cmd := opts.newSetupCmd()

	assert.NotNil(t, cmd)
	assert.Equal(t, "setup", cmd.Use)
	assert.Equal(t, "Interactive setup for KubeArchive configuration", cmd.Short)
	assert.True(t, cmd.SilenceUsage)
}
