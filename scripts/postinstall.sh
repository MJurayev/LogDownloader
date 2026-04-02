#!/bin/sh
if ! id logdownloader >/dev/null 2>&1; then
  useradd -r -s /sbin/nologin logdownloader
fi
mkdir -p /var/lib/logdownloader
chown logdownloader:logdownloader /var/lib/logdownloader
systemctl daemon-reload
systemctl enable logdownloader
systemctl start logdownloader
