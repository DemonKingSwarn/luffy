#!/usr/bin/env bash

podman run -it --rm \
  -v "$PWD:/work" \
  -w /work \
  nixos/nix:latest \
  bash
