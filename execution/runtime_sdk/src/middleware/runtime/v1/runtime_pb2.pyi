from google.protobuf import struct_pb2 as _struct_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class RuntimeType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    RUNTIME_TYPE_UNSPECIFIED: _ClassVar[RuntimeType]
    NORMAL: _ClassVar[RuntimeType]
    SANDBOX: _ClassVar[RuntimeType]
    LONGTERM: _ClassVar[RuntimeType]

class ToolScope(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    TOOL_SCOPE_UNSPECIFIED: _ClassVar[ToolScope]
    LOCAL: _ClassVar[ToolScope]
    GLOBAL: _ClassVar[ToolScope]

class EventType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVENT_TYPE_UNSPECIFIED: _ClassVar[EventType]
    RUN_STATUS: _ClassVar[EventType]
    MESSAGE_DELTA: _ClassVar[EventType]
    MESSAGE_COMPLETED: _ClassVar[EventType]
    TOOL_CALL_BEGIN: _ClassVar[EventType]
    TOOL_CALL_END: _ClassVar[EventType]
    USAGE: _ClassVar[EventType]
    ERROR: _ClassVar[EventType]
    CHECKPOINT_SAVED: _ClassVar[EventType]
    CUSTOM: _ClassVar[EventType]
RUNTIME_TYPE_UNSPECIFIED: RuntimeType
NORMAL: RuntimeType
SANDBOX: RuntimeType
LONGTERM: RuntimeType
TOOL_SCOPE_UNSPECIFIED: ToolScope
LOCAL: ToolScope
GLOBAL: ToolScope
EVENT_TYPE_UNSPECIFIED: EventType
RUN_STATUS: EventType
MESSAGE_DELTA: EventType
MESSAGE_COMPLETED: EventType
TOOL_CALL_BEGIN: EventType
TOOL_CALL_END: EventType
USAGE: EventType
ERROR: EventType
CHECKPOINT_SAVED: EventType
CUSTOM: EventType

class Envelope(_message.Message):
    __slots__ = ("msg_id", "correlation_id", "run_id", "ts_ms", "attempt", "lease_epoch", "hello", "heartbeat", "task_assign", "task_ack", "event", "tool_call_request", "tool_call_result", "cancel", "reconcile", "checkpoint", "error")
    MSG_ID_FIELD_NUMBER: _ClassVar[int]
    CORRELATION_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    TS_MS_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_FIELD_NUMBER: _ClassVar[int]
    LEASE_EPOCH_FIELD_NUMBER: _ClassVar[int]
    HELLO_FIELD_NUMBER: _ClassVar[int]
    HEARTBEAT_FIELD_NUMBER: _ClassVar[int]
    TASK_ASSIGN_FIELD_NUMBER: _ClassVar[int]
    TASK_ACK_FIELD_NUMBER: _ClassVar[int]
    EVENT_FIELD_NUMBER: _ClassVar[int]
    TOOL_CALL_REQUEST_FIELD_NUMBER: _ClassVar[int]
    TOOL_CALL_RESULT_FIELD_NUMBER: _ClassVar[int]
    CANCEL_FIELD_NUMBER: _ClassVar[int]
    RECONCILE_FIELD_NUMBER: _ClassVar[int]
    CHECKPOINT_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    msg_id: str
    correlation_id: str
    run_id: str
    ts_ms: int
    attempt: int
    lease_epoch: int
    hello: Hello
    heartbeat: Heartbeat
    task_assign: TaskAssign
    task_ack: TaskAck
    event: Event
    tool_call_request: ToolCallRequest
    tool_call_result: ToolCallResult
    cancel: Cancel
    reconcile: Reconcile
    checkpoint: Checkpoint
    error: ErrorReport
    def __init__(self, msg_id: _Optional[str] = ..., correlation_id: _Optional[str] = ..., run_id: _Optional[str] = ..., ts_ms: _Optional[int] = ..., attempt: _Optional[int] = ..., lease_epoch: _Optional[int] = ..., hello: _Optional[_Union[Hello, _Mapping]] = ..., heartbeat: _Optional[_Union[Heartbeat, _Mapping]] = ..., task_assign: _Optional[_Union[TaskAssign, _Mapping]] = ..., task_ack: _Optional[_Union[TaskAck, _Mapping]] = ..., event: _Optional[_Union[Event, _Mapping]] = ..., tool_call_request: _Optional[_Union[ToolCallRequest, _Mapping]] = ..., tool_call_result: _Optional[_Union[ToolCallResult, _Mapping]] = ..., cancel: _Optional[_Union[Cancel, _Mapping]] = ..., reconcile: _Optional[_Union[Reconcile, _Mapping]] = ..., checkpoint: _Optional[_Union[Checkpoint, _Mapping]] = ..., error: _Optional[_Union[ErrorReport, _Mapping]] = ...) -> None: ...

class Hello(_message.Message):
    __slots__ = ("daemon_id", "auth_token", "runtime_type", "max_concurrent_runs", "version", "labels", "running_runs", "runtime_name", "capabilities")
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
    AUTH_TOKEN_FIELD_NUMBER: _ClassVar[int]
    RUNTIME_TYPE_FIELD_NUMBER: _ClassVar[int]
    MAX_CONCURRENT_RUNS_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    LABELS_FIELD_NUMBER: _ClassVar[int]
    RUNNING_RUNS_FIELD_NUMBER: _ClassVar[int]
    RUNTIME_NAME_FIELD_NUMBER: _ClassVar[int]
    CAPABILITIES_FIELD_NUMBER: _ClassVar[int]
    daemon_id: str
    auth_token: str
    runtime_type: RuntimeType
    max_concurrent_runs: int
    version: str
    labels: _containers.ScalarMap[str, str]
    running_runs: _containers.RepeatedCompositeFieldContainer[RunningRun]
    runtime_name: str
    capabilities: _containers.ScalarMap[str, str]
    def __init__(self, daemon_id: _Optional[str] = ..., auth_token: _Optional[str] = ..., runtime_type: _Optional[_Union[RuntimeType, str]] = ..., max_concurrent_runs: _Optional[int] = ..., version: _Optional[str] = ..., labels: _Optional[_Mapping[str, str]] = ..., running_runs: _Optional[_Iterable[_Union[RunningRun, _Mapping]]] = ..., runtime_name: _Optional[str] = ..., capabilities: _Optional[_Mapping[str, str]] = ...) -> None: ...

class RunningRun(_message.Message):
    __slots__ = ("run_id", "lease_epoch")
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    LEASE_EPOCH_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    lease_epoch: int
    def __init__(self, run_id: _Optional[str] = ..., lease_epoch: _Optional[int] = ...) -> None: ...

class Reconcile(_message.Message):
    __slots__ = ("cancel_run_ids",)
    CANCEL_RUN_IDS_FIELD_NUMBER: _ClassVar[int]
    cancel_run_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, cancel_run_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class Heartbeat(_message.Message):
    __slots__ = ("running_count", "cpu_load", "mem_mb")
    RUNNING_COUNT_FIELD_NUMBER: _ClassVar[int]
    CPU_LOAD_FIELD_NUMBER: _ClassVar[int]
    MEM_MB_FIELD_NUMBER: _ClassVar[int]
    running_count: int
    cpu_load: float
    mem_mb: float
    def __init__(self, running_count: _Optional[int] = ..., cpu_load: _Optional[float] = ..., mem_mb: _Optional[float] = ...) -> None: ...

class TaskAssign(_message.Message):
    __slots__ = ("run_id", "app_id", "params", "input", "model", "tools", "mcp_bindings", "timeouts", "session_id", "resume_from", "trace_context")
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    APP_ID_FIELD_NUMBER: _ClassVar[int]
    PARAMS_FIELD_NUMBER: _ClassVar[int]
    INPUT_FIELD_NUMBER: _ClassVar[int]
    MODEL_FIELD_NUMBER: _ClassVar[int]
    TOOLS_FIELD_NUMBER: _ClassVar[int]
    MCP_BINDINGS_FIELD_NUMBER: _ClassVar[int]
    TIMEOUTS_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    RESUME_FROM_FIELD_NUMBER: _ClassVar[int]
    TRACE_CONTEXT_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    app_id: str
    params: _struct_pb2.Struct
    input: _containers.RepeatedCompositeFieldContainer[ChatMessage]
    model: ModelProfile
    tools: _containers.RepeatedCompositeFieldContainer[ToolManifest]
    mcp_bindings: _containers.RepeatedCompositeFieldContainer[McpBinding]
    timeouts: TimeoutProfile
    session_id: str
    resume_from: Checkpoint
    trace_context: str
    def __init__(self, run_id: _Optional[str] = ..., app_id: _Optional[str] = ..., params: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., input: _Optional[_Iterable[_Union[ChatMessage, _Mapping]]] = ..., model: _Optional[_Union[ModelProfile, _Mapping]] = ..., tools: _Optional[_Iterable[_Union[ToolManifest, _Mapping]]] = ..., mcp_bindings: _Optional[_Iterable[_Union[McpBinding, _Mapping]]] = ..., timeouts: _Optional[_Union[TimeoutProfile, _Mapping]] = ..., session_id: _Optional[str] = ..., resume_from: _Optional[_Union[Checkpoint, _Mapping]] = ..., trace_context: _Optional[str] = ...) -> None: ...

class ChatMessage(_message.Message):
    __slots__ = ("role", "content", "name")
    ROLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    role: str
    content: str
    name: str
    def __init__(self, role: _Optional[str] = ..., content: _Optional[str] = ..., name: _Optional[str] = ...) -> None: ...

class ModelProfile(_message.Message):
    __slots__ = ("provider", "model", "api_key", "base_url", "params")
    class ParamsEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    PROVIDER_FIELD_NUMBER: _ClassVar[int]
    MODEL_FIELD_NUMBER: _ClassVar[int]
    API_KEY_FIELD_NUMBER: _ClassVar[int]
    BASE_URL_FIELD_NUMBER: _ClassVar[int]
    PARAMS_FIELD_NUMBER: _ClassVar[int]
    provider: str
    model: str
    api_key: str
    base_url: str
    params: _containers.ScalarMap[str, str]
    def __init__(self, provider: _Optional[str] = ..., model: _Optional[str] = ..., api_key: _Optional[str] = ..., base_url: _Optional[str] = ..., params: _Optional[_Mapping[str, str]] = ...) -> None: ...

class ToolManifest(_message.Message):
    __slots__ = ("name", "description", "input_schema_json", "scope")
    NAME_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    INPUT_SCHEMA_JSON_FIELD_NUMBER: _ClassVar[int]
    SCOPE_FIELD_NUMBER: _ClassVar[int]
    name: str
    description: str
    input_schema_json: str
    scope: ToolScope
    def __init__(self, name: _Optional[str] = ..., description: _Optional[str] = ..., input_schema_json: _Optional[str] = ..., scope: _Optional[_Union[ToolScope, str]] = ...) -> None: ...

class McpBinding(_message.Message):
    __slots__ = ("server_name", "transport", "command", "args", "env", "url", "headers", "allowed_tools", "namespace")
    class EnvEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    class HeadersEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    SERVER_NAME_FIELD_NUMBER: _ClassVar[int]
    TRANSPORT_FIELD_NUMBER: _ClassVar[int]
    COMMAND_FIELD_NUMBER: _ClassVar[int]
    ARGS_FIELD_NUMBER: _ClassVar[int]
    ENV_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    HEADERS_FIELD_NUMBER: _ClassVar[int]
    ALLOWED_TOOLS_FIELD_NUMBER: _ClassVar[int]
    NAMESPACE_FIELD_NUMBER: _ClassVar[int]
    server_name: str
    transport: str
    command: str
    args: _containers.RepeatedScalarFieldContainer[str]
    env: _containers.ScalarMap[str, str]
    url: str
    headers: _containers.ScalarMap[str, str]
    allowed_tools: _containers.RepeatedScalarFieldContainer[str]
    namespace: str
    def __init__(self, server_name: _Optional[str] = ..., transport: _Optional[str] = ..., command: _Optional[str] = ..., args: _Optional[_Iterable[str]] = ..., env: _Optional[_Mapping[str, str]] = ..., url: _Optional[str] = ..., headers: _Optional[_Mapping[str, str]] = ..., allowed_tools: _Optional[_Iterable[str]] = ..., namespace: _Optional[str] = ...) -> None: ...

class TimeoutProfile(_message.Message):
    __slots__ = ("run_timeout_ms", "tool_timeout_ms")
    RUN_TIMEOUT_MS_FIELD_NUMBER: _ClassVar[int]
    TOOL_TIMEOUT_MS_FIELD_NUMBER: _ClassVar[int]
    run_timeout_ms: int
    tool_timeout_ms: int
    def __init__(self, run_timeout_ms: _Optional[int] = ..., tool_timeout_ms: _Optional[int] = ...) -> None: ...

class TaskAck(_message.Message):
    __slots__ = ("accepted", "reason")
    ACCEPTED_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    accepted: bool
    reason: str
    def __init__(self, accepted: _Optional[bool] = ..., reason: _Optional[str] = ...) -> None: ...

class Event(_message.Message):
    __slots__ = ("seq", "type", "ephemeral", "data", "source", "name", "schema_version")
    SEQ_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    EPHEMERAL_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    SOURCE_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    seq: int
    type: EventType
    ephemeral: bool
    data: _struct_pb2.Struct
    source: str
    name: str
    schema_version: str
    def __init__(self, seq: _Optional[int] = ..., type: _Optional[_Union[EventType, str]] = ..., ephemeral: _Optional[bool] = ..., data: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., source: _Optional[str] = ..., name: _Optional[str] = ..., schema_version: _Optional[str] = ...) -> None: ...

class ToolCallRequest(_message.Message):
    __slots__ = ("invocation_id", "tool_name", "arguments_json", "timeout_ms")
    INVOCATION_ID_FIELD_NUMBER: _ClassVar[int]
    TOOL_NAME_FIELD_NUMBER: _ClassVar[int]
    ARGUMENTS_JSON_FIELD_NUMBER: _ClassVar[int]
    TIMEOUT_MS_FIELD_NUMBER: _ClassVar[int]
    invocation_id: str
    tool_name: str
    arguments_json: str
    timeout_ms: int
    def __init__(self, invocation_id: _Optional[str] = ..., tool_name: _Optional[str] = ..., arguments_json: _Optional[str] = ..., timeout_ms: _Optional[int] = ...) -> None: ...

class ToolCallResult(_message.Message):
    __slots__ = ("invocation_id", "ok", "result_json", "error")
    INVOCATION_ID_FIELD_NUMBER: _ClassVar[int]
    OK_FIELD_NUMBER: _ClassVar[int]
    RESULT_JSON_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    invocation_id: str
    ok: bool
    result_json: str
    error: ErrorInfo
    def __init__(self, invocation_id: _Optional[str] = ..., ok: _Optional[bool] = ..., result_json: _Optional[str] = ..., error: _Optional[_Union[ErrorInfo, _Mapping]] = ...) -> None: ...

class ErrorInfo(_message.Message):
    __slots__ = ("code", "message", "retryable")
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    RETRYABLE_FIELD_NUMBER: _ClassVar[int]
    code: str
    message: str
    retryable: bool
    def __init__(self, code: _Optional[str] = ..., message: _Optional[str] = ..., retryable: _Optional[bool] = ...) -> None: ...

class Cancel(_message.Message):
    __slots__ = ("reason",)
    REASON_FIELD_NUMBER: _ClassVar[int]
    reason: str
    def __init__(self, reason: _Optional[str] = ...) -> None: ...

class Checkpoint(_message.Message):
    __slots__ = ("session_id", "event_seq_watermark", "state")
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    EVENT_SEQ_WATERMARK_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    session_id: str
    event_seq_watermark: int
    state: bytes
    def __init__(self, session_id: _Optional[str] = ..., event_seq_watermark: _Optional[int] = ..., state: _Optional[bytes] = ...) -> None: ...

class ErrorReport(_message.Message):
    __slots__ = ("code", "message")
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    code: str
    message: str
    def __init__(self, code: _Optional[str] = ..., message: _Optional[str] = ...) -> None: ...
