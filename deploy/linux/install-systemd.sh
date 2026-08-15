#!/usr/bin/env sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Ejecuta este instalador como root." >&2
  exit 1
fi

BINARY=${1:-./personalcloud}
if [ ! -f "$BINARY" ]; then
  echo "No existe el binario: $BINARY" >&2
  exit 1
fi

if ! getent group disk >/dev/null 2>&1; then
  echo "No existe el grupo disk; crea una política de acceso a block devices apropiada para tu distribución." >&2
  exit 1
fi

if ! id personalcloud >/dev/null 2>&1; then
  useradd --system --home /var/lib/personalcloud --shell /usr/sbin/nologin personalcloud
fi
usermod -a -G disk personalcloud
install -d -m 0700 -o personalcloud -g personalcloud /var/lib/personalcloud /var/lib/personalcloud/data /mnt/personalcloud
install -d -m 0750 /etc/personalcloud
install -m 0755 "$BINARY" /usr/local/bin/personalcloud
install -m 0644 deploy/linux/personalcloud.service /etc/systemd/system/personalcloud.service
if [ ! -f /etc/personalcloud/personalcloud.env ]; then
  install -m 0600 deploy/linux/personalcloud.env.example /etc/personalcloud/personalcloud.env
fi
systemctl daemon-reload
systemctl enable --now personalcloud.service
systemctl --no-pager --full status personalcloud.service || true
