# APIMeter HA deployment assets

These files are topology-specific production examples, not a complete HA installer. Review and adapt the addresses, interfaces, upstreams, and paths before installing them on a server.

Before enabling `apimeter-log-collector.service`, install the collector and its configuration at:

```text
/opt/apimeter-log-collector/collector.py
```

The unit has a `ConditionPathIsFile` guard so a missing collector is reported as an unmet condition instead of entering a restart loop.

Before installing the WireGuard proxy units, configure `wg0` and confirm that `10.38.145.2` belongs to the local host. Then install and verify the units explicitly:

```bash
sudo install -m 0644 scripts/ha/apimeter-openobserve-wg.socket /etc/systemd/system/
sudo install -m 0644 scripts/ha/apimeter-openobserve-wg.service /etc/systemd/system/
sudo install -m 0644 scripts/ha/apimeter-log-collector.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now apimeter-openobserve-wg.socket
sudo systemctl enable --now apimeter-log-collector.service
sudo systemctl status apimeter-openobserve-wg.socket apimeter-log-collector.service
```

Validate `Caddyfile.38` with `caddy validate` after replacing its example topology values and before reloading the active Caddy configuration.
