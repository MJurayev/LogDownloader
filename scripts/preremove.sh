#!/bin/sh
systemctl stop logdownloader || true
systemctl disable logdownloader || true
