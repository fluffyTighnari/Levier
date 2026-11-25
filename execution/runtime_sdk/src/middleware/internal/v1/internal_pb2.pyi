from google.protobuf import empty_pb2 as _empty_pb2
from middleware.runtime.v1 import runtime_pb2 as _runtime_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class DispatchRequest(_message.Message):
    __slots__ = ("daemon_id", "envelope", "ack_timeout_ms")
    DAEMON_ID_FIELD_NUMBER: _ClassVar[int]
    ENVELOPE_FIELD_NUMBER: _ClassVar[int]
    ACK_TIMEOUT_MS_FIELD_NUMBER: _ClassVar[int]
    daemon_id: str
    envelope: _runtime_pb2.Envelope
    ack_timeout_ms: int
    def __init__(self, daemon_id: _Optional[str] = ..., envelope: _Optional[_Union[_runtime_pb2.Envelope, _Mapping]] = ..., ack_timeout_ms: _Optional[int] = ...) -> None: ...

class DispatchResponse(_message.Message):
    __slots__ = ("accepted", "reason", "ack")
    ACCEPTED_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    ACK_FIELD_NUMBER: _ClassVar[int]
    accepted: bool
    reason: str
    ack: _runtime_pb2.TaskAck
    def __init__(self, accepted: _Optional[bool] = ..., reason: _Optional[str] = ..., ack: _Optional[_Union[_runtime_pb2.TaskAck, _Mapping]] = ...) -> None: ...

class SendRequest(_message.Message):
    __slots__ = ("daemon_id", "envelope", "connection_epoch")
    DAEMON_ID_FIELD_NUMBER: _ClassVar[int]
    ENVELOPE_FIELD_NUMBER: _ClassVar[int]
    CONNECTION_EPOCH_FIELD_NUMBER: _ClassVar[int]
    daemon_id: str
    envelope: _runtime_pb2.Envelope
    connection_epoch: int
    def __init__(self, daemon_id: _Optional[str] = ..., envelope: _Optional[_Union[_runtime_pb2.Envelope, _Mapping]] = ..., connection_epoch: _Optional[int] = ...) -> None: ...

class ReserveRequest(_message.Message):
    __slots__ = ("daemon_id", "reservation_id", "ttl_ms", "run_id")
    DAEMON_ID_FIELD_NUMBER: _ClassVar[int]
    RESERVATION_ID_FIELD_NUMBER: _ClassVar[int]
    TTL_MS_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    daemon_id: str
    reservation_id: str
    ttl_ms: int
    run_id: str
    def __init__(self, daemon_id: _Optional[str] = ..., reservation_id: _Optional[str] = ..., ttl_ms: _Optional[int] = ..., run_id: _Optional[str] = ...) -> None: ...

class ReserveResponse(_message.Message):
    __slots__ = ("accepted", "reason")
    ACCEPTED_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    accepted: bool
    reason: str
    def __init__(self, accepted: _Optional[bool] = ..., reason: _Optional[str] = ...) -> None: ...

class ReleaseRequest(_message.Message):
    __slots__ = ("daemon_id", "reservation_id")
    DAEMON_ID_FIELD_NUMBER: _ClassVar[int]
    RESERVATION_ID_FIELD_NUMBER: _ClassVar[int]
    daemon_id: str
    reservation_id: str
    def __init__(self, daemon_id: _Optional[str] = ..., reservation_id: _Optional[str] = ...) -> None: ...

class DisconnectRequest(_message.Message):
    __slots__ = ("daemon_id", "connection_epoch")
    DAEMON_ID_FIELD_NUMBER: _ClassVar[int]
    CONNECTION_EPOCH_FIELD_NUMBER: _ClassVar[int]
    daemon_id: str
    connection_epoch: int
    def __init__(self, daemon_id: _Optional[str] = ..., connection_epoch: _Optional[int] = ...) -> None: ...

class ListRuntimesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class RuntimeSnapshot(_message.Message):
    __slots__ = ("daemon_id", "gateway_id", "connection_epoch", "runtime_name", "runtime_type", "max_concurrent_runs", "running_count", "reserved", "labels", "capabilities")
    class LabelsEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    class CapabilitiesEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    DAEMON_ID_FIELD_NUMBER: _ClassVar[int]
    GATEWAY_ID_FIELD_NUMBER: _ClassVar[int]
    CONNECTION_EPOCH_FIELD_NUMBER: _ClassVar[int]
    RUNTIME_NAME_FIELD_NUMBER: _ClassVar[int]
    RUNTIME_TYPE_FIELD_NUMBER: _ClassVar[int]
    MAX_CONCURRENT_RUNS_FIELD_NUMBER: _ClassVar[int]
    RUNNING_COUNT_FIELD_NUMBER: _ClassVar[int]
    RESERVED_FIELD_NUMBER: _ClassVar[int]
    LABELS_FIELD_NUMBER: _ClassVar[int]
    CAPABILITIES_FIELD_NUMBER: _ClassVar[int]
    daemon_id: str
    gateway_id: str
    connection_epoch: int
    runtime_name: str
    runtime_type: str
    max_concurrent_runs: int
    running_count: int
    reserved: int
    labels: _containers.ScalarMap[str, str]
    capabilities: _containers.ScalarMap[str, str]
    def __init__(self, daemon_id: _Optional[str] = ..., gateway_id: _Optional[str] = ..., connection_epoch: _Optional[int] = ..., runtime_name: _Optional[str] = ..., runtime_type: _Optional[str] = ..., max_concurrent_runs: _Optional[int] = ..., running_count: _Optional[int] = ..., reserved: _Optional[int] = ..., labels: _Optional[_Mapping[str, str]] = ..., capabilities: _Optional[_Mapping[str, str]] = ...) -> None: ...

class ListRuntimesResponse(_message.Message):
    __slots__ = ("runtimes",)
    RUNTIMES_FIELD_NUMBER: _ClassVar[int]
    runtimes: _containers.RepeatedCompositeFieldContainer[RuntimeSnapshot]
    def __init__(self, runtimes: _Optional[_Iterable[_Union[RuntimeSnapshot, _Mapping]]] = ...) -> None: ...

class DeliverEnvelopeRequest(_message.Message):
    __slots__ = ("gateway_id", "daemon_id", "envelope")
    GATEWAY_ID_FIELD_NUMBER: _ClassVar[int]
    DAEMON_ID_FIELD_NUMBER: _ClassVar[int]
    ENVELOPE_FIELD_NUMBER: _ClassVar[int]
    gateway_id: str
    daemon_id: str
    envelope: _runtime_pb2.Envelope
    def __init__(self, gateway_id: _Optional[str] = ..., daemon_id: _Optional[str] = ..., envelope: _Optional[_Union[_runtime_pb2.Envelope, _Mapping]] = ...) -> None: ...

class CancelRunRequest(_message.Message):
    __slots__ = ("run_id", "reason")
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    reason: str
    def __init__(self, run_id: _Optional[str] = ..., reason: _Optional[str] = ...) -> None: ...

class CancelRunResponse(_message.Message):
    __slots__ = ("run_id", "status")
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    status: str
    def __init__(self, run_id: _Optional[str] = ..., status: _Optional[str] = ...) -> None: ...

class ResumeRunRequest(_message.Message):
    __slots__ = ("run_id", "resume_value_json")
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    RESUME_VALUE_JSON_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    resume_value_json: bytes
    def __init__(self, run_id: _Optional[str] = ..., resume_value_json: _Optional[bytes] = ...) -> None: ...

class ResumeRunResponse(_message.Message):
    __slots__ = ("run_id", "session_id", "status")
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    session_id: str
    status: str
    def __init__(self, run_id: _Optional[str] = ..., session_id: _Optional[str] = ..., status: _Optional[str] = ...) -> None: ...

class CallLocalToolRequest(_message.Message):
    __slots__ = ("app_id", "connection_id", "request", "timeout_ms")
    APP_ID_FIELD_NUMBER: _ClassVar[int]
    CONNECTION_ID_FIELD_NUMBER: _ClassVar[int]
    REQUEST_FIELD_NUMBER: _ClassVar[int]
    TIMEOUT_MS_FIELD_NUMBER: _ClassVar[int]
    app_id: str
    connection_id: str
    request: _runtime_pb2.ToolCallRequest
    timeout_ms: int
    def __init__(self, app_id: _Optional[str] = ..., connection_id: _Optional[str] = ..., request: _Optional[_Union[_runtime_pb2.ToolCallRequest, _Mapping]] = ..., timeout_ms: _Optional[int] = ...) -> None: ...

class CallLocalToolResponse(_message.Message):
    __slots__ = ("result",)
    RESULT_FIELD_NUMBER: _ClassVar[int]
    result: _runtime_pb2.ToolCallResult
    def __init__(self, result: _Optional[_Union[_runtime_pb2.ToolCallResult, _Mapping]] = ...) -> None: ...

class CancelLocalToolsRequest(_message.Message):
    __slots__ = ("app_id", "run_id", "code", "message")
    APP_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    app_id: str
    run_id: str
    code: str
    message: str
    def __init__(self, app_id: _Optional[str] = ..., run_id: _Optional[str] = ..., code: _Optional[str] = ..., message: _Optional[str] = ...) -> None: ...

class PublishRunEventRequest(_message.Message):
    __slots__ = ("app_id", "run_id", "payload")
    APP_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    app_id: str
    run_id: str
    payload: bytes
    def __init__(self, app_id: _Optional[str] = ..., run_id: _Optional[str] = ..., payload: _Optional[bytes] = ...) -> None: ...

class PublishRunCompletedRequest(_message.Message):
    __slots__ = ("app_id", "run_id", "status", "attempt")
    APP_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_FIELD_NUMBER: _ClassVar[int]
    app_id: str
    run_id: str
    status: str
    attempt: int
    def __init__(self, app_id: _Optional[str] = ..., run_id: _Optional[str] = ..., status: _Optional[str] = ..., attempt: _Optional[int] = ...) -> None: ...
