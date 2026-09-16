# Copyright IBM Corp. 2026
# SPDX-License-Identifier: MPL-2.0

terraform {
  required_version = ">= 1.4.0"
}

resource "terraform_data" "e2e" {
  input = "tfctl-e2e"
}
