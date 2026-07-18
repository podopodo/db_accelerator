# V0.0.2 — transparent MySQL protocol path

Goal: connect a native client to accelerator, execute through one upstream connection, and receive a faithful result.

## Tasks

- [x] [01 — packet codec and listener](01.task_packet_codec_listener.md)
- [ ] [02 — handshake and capabilities](02.task_handshake_capabilities.md)
- [ ] [03 — upstream connector](03.task_upstream_connector.md)
- [ ] [04 — query and result relay](04.task_query_result_relay.md)
- [ ] [05 — TLS and authentication](05.task_tls_authentication.md)
- [ ] [06 — differential protocol gate](06.task_differential_protocol_gate.md)

## Exit gate

- [ ] Supported client connects using native driver.
- [ ] Basic queries match direct database results and errors.
- [ ] TLS and credential handling pass threat-model checks.
- [ ] Unsupported capabilities fail clearly.
