// Copyright (c) 2023 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGlobalAuthCheck_NoSet(t *testing.T) {
	c := JWTAuthConfig{}
	require.Equal(t, GlobalAuthNoSet, c.GlobalAuthCheck())
}

func TestGlobalAuthCheck_True(t *testing.T) {
	tr := true
	c := JWTAuthConfig{GlobalAuth: &tr}
	require.Equal(t, GlobalAuthTrue, c.GlobalAuthCheck())
}

func TestGlobalAuthCheck_False(t *testing.T) {
	fa := false
	c := JWTAuthConfig{GlobalAuth: &fa}
	require.Equal(t, GlobalAuthFalse, c.GlobalAuthCheck())
}
