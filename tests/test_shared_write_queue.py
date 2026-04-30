import queue
import threading

import pytest

from shared.write_queue import (
    _WRITE_STOP,
    _ensure_write_worker,
    _queue_write,
    _run_write_batch,
    _shutdown_write_worker,
    _write_queue_depth,
    _write_worker_main,
    _WriteTask,
)


class _FakeDb:
    def __init__(self):
        self.calls = []
        self.commits = 0

    def commit(self):
        self.commits += 1


class _FakeThread:
    def __init__(self, alive=True):
        self._alive = alive
        self.started = False
        self.join_calls = []

    def is_alive(self):
        return self._alive

    def start(self):
        self.started = True
        self._alive = True

    def join(self, timeout=None):
        self.join_calls.append(timeout)


class _FakeQueue:
    def __init__(self, items=None, fail_put=False):
        self.items = list(items or [])
        self.fail_put = fail_put
        self.put_calls = []

    def get(self, timeout=None):
        if not self.items:
            raise queue.Empty()
        item = self.items.pop(0)
        if item is queue.Empty:
            raise queue.Empty()
        return item

    def put(self, item, timeout=None):
        self.put_calls.append((item, timeout))
        if self.fail_put:
            raise queue.Full()
        self.items.append(item)

    def qsize(self):
        return len(self.items)


class _FakeEvent:
    def __init__(self):
        self.set_called = False
        self.wait_calls = []

    def set(self):
        self.set_called = True

    def wait(self, timeout=None):
        self.wait_calls.append(timeout)
        return True


def test_shared_write_queue_run_write_batch_commits_sets_done_and_captures_errors():
    db = _FakeDb()
    event_ok = _FakeEvent()
    event_error = _FakeEvent()

    def _ok_op(current_db):
        current_db.calls.append("ok")

    def _bad_op(_current_db):
        raise RuntimeError("boom")

    tasks = [
        _WriteTask(op=_ok_op, done=event_ok),
        _WriteTask(op=_bad_op, done=event_error),
    ]

    _run_write_batch(tasks, get_db=lambda: db)

    assert db.calls == ["ok"]
    assert db.commits == 1
    assert event_ok.set_called is True
    assert event_error.set_called is True
    assert isinstance(tasks[1].error, RuntimeError)


def test_shared_write_queue_worker_main_returns_immediately_on_stop():
    fake_queue = _FakeQueue(items=[_WRITE_STOP])
    batches = []

    _write_worker_main(
        write_queue=fake_queue,
        write_stop=_WRITE_STOP,
        batch_wait_ms=20,
        batch_max=2,
        run_write_batch=lambda batch: batches.append(batch),
    )

    assert batches == []


def test_shared_write_queue_worker_main_flushes_batch_before_stop():
    task = _WriteTask(op=lambda _db: None)
    fake_queue = _FakeQueue(items=[task, _WRITE_STOP])
    batches = []

    _write_worker_main(
        write_queue=fake_queue,
        write_stop=_WRITE_STOP,
        batch_wait_ms=20,
        batch_max=4,
        run_write_batch=lambda batch: batches.append(batch),
    )

    assert batches == [[task]]


def test_shared_write_queue_worker_main_handles_queue_empty_and_deadline_paths():
    task = _WriteTask(op=lambda _db: None)
    empty_queue = _FakeQueue(items=[task, queue.Empty, _WRITE_STOP])
    batches = []

    _write_worker_main(
        write_queue=empty_queue,
        write_stop=_WRITE_STOP,
        batch_wait_ms=20,
        batch_max=4,
        run_write_batch=lambda batch: batches.append(batch),
        monotonic=lambda: 0.0,
    )

    assert batches == [[task]]

    deadline_queue = _FakeQueue(items=[task, _WRITE_STOP])
    deadline_batches = []
    monotonic_values = iter([0.0, 1.0])

    _write_worker_main(
        write_queue=deadline_queue,
        write_stop=_WRITE_STOP,
        batch_wait_ms=1,
        batch_max=4,
        run_write_batch=lambda batch: deadline_batches.append(batch),
        monotonic=lambda: next(monotonic_values),
    )

    assert deadline_batches == [[task]]


def test_shared_write_queue_ensure_reuses_alive_thread_and_starts_missing_worker():
    existing_queue = _FakeQueue()
    existing_thread = _FakeThread(alive=True)
    returned_queue, returned_thread = _ensure_write_worker(
        write_queue=existing_queue,
        write_thread=existing_thread,
        write_worker_lock=threading.Lock(),
        write_queue_max=5,
        worker_target=lambda: None,
    )
    assert returned_queue is existing_queue
    assert returned_thread is existing_thread

    started = {}

    def _thread_factory(*, target, name, daemon):
        started["target"] = target
        started["name"] = name
        started["daemon"] = daemon
        return _FakeThread(alive=False)

    created_queue, created_thread = _ensure_write_worker(
        write_queue=None,
        write_thread=None,
        write_worker_lock=threading.Lock(),
        write_queue_max=7,
        worker_target=lambda: None,
        thread_factory=_thread_factory,
    )
    assert created_queue.maxsize == 7
    assert created_thread.started is True
    assert callable(started["target"])
    assert started["name"] == "sobs-db-writer"
    assert started["daemon"] is True


def test_shared_write_queue_enqueue_depth_and_wait_error_paths():
    queued = []
    fake_queue = _FakeQueue()

    def _ensure():
        return None

    def _get_queue():
        return fake_queue

    _queue_write(lambda _db: None, ensure_write_worker=_ensure, get_write_queue=_get_queue)
    assert len(fake_queue.items) == 1
    assert _write_queue_depth(fake_queue) == 1
    assert _write_queue_depth(None) == 0

    class _QueueFullError(RuntimeError):
        pass

    with pytest.raises(_QueueFullError):
        _queue_write(
            lambda _db: None,
            ensure_write_worker=_ensure,
            get_write_queue=lambda: _FakeQueue(fail_put=True),
            write_queue_full_error_cls=_QueueFullError,
        )

    wait_event = _FakeEvent()

    def _put_error(task, timeout=None):
        queued.append((task, timeout))
        task.error = ValueError("write failed")

    error_queue = _FakeQueue()
    error_queue.put = _put_error
    with pytest.raises(ValueError, match="write failed"):
        _queue_write(
            lambda _db: None,
            ensure_write_worker=_ensure,
            get_write_queue=lambda: error_queue,
            wait=True,
            event_factory=lambda: wait_event,
        )
    assert wait_event.wait_calls == [15]


def test_shared_write_queue_shutdown_signals_and_joins_worker():
    fake_queue = _FakeQueue()
    fake_thread = _FakeThread(alive=True)

    assert _shutdown_write_worker(
        write_queue=fake_queue,
        write_thread=fake_thread,
        write_worker_lock=threading.Lock(),
        write_stop=_WRITE_STOP,
    ) == (None, None)
    assert fake_queue.put_calls == [(_WRITE_STOP, 1)]
    assert fake_thread.join_calls == [5]

    full_queue = _FakeQueue(fail_put=True)
    full_thread = _FakeThread(alive=True)
    _shutdown_write_worker(
        write_queue=full_queue,
        write_thread=full_thread,
        write_worker_lock=threading.Lock(),
        write_stop=_WRITE_STOP,
    )
    assert full_thread.join_calls == [5]
