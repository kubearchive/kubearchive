#!/bin/bash
# Copyright Kronicler Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o xtrace

kubectl delete -k config/
