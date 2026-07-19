# V0.0.2 — transparent MySQL protocol path

Goal: connect a native client to accelerator, execute through one upstream connection, and receive a faithful result.

## Tasks

- [x] [01 — packet codec and listener](01.task_packet_codec_listener.md)
- [x] [02 — handshake and capabilities](02.task_handshake_capabilities.md)
- [x] [03 — upstream connector](03.task_upstream_connector.md)
- [x] [04 — query and result relay](04.task_query_result_relay.md)
- [x] [05 — TLS and authentication](05.task_tls_authentication.md)
- [x] [06 — differential protocol gate](06.task_differential_protocol_gate.md)

## Exit gate

- [x] Supported client connects using native driver.
- [x] Basic queries match direct database results and errors.
- [x] TLS and credential handling pass threat-model checks.
- [x] Unsupported capabilities fail clearly.
