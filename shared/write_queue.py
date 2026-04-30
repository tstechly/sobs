"""Shared write queue helpers used by SOBS ingest paths."""

from __future__ import annotations

import queue
import threading
import time
from dataclasses import dataclass
from typing import Any, Callable, cast


@dataclass
class _WriteTask:
    op: Callable[[Any], None]
    done: threading.Event | None = None
    error: Exception | None = None


_WRITE_STOP = object()


def _run_write_batch(tasks: list[_WriteTask], *, get_db) -> None:
    db = get_db()
    for task in tasks:
        try:
            task.op(db)
        except Exception as exc:
            task.error = exc
    db.commit()
    for task in tasks:
        if task.done is not None:
            task.done.set()


def _write_worker_main(
    *,
    write_queue,
    write_stop,
    batch_wait_ms: int,
    batch_max: int,
    run_write_batch,
    monotonic=time.monotonic,
) -> None:
    while True:
        first = write_queue.get()
        if first is write_stop:
            return
        batch = [cast(_WriteTask, first)]
        deadline = monotonic() + (max(1, batch_wait_ms) / 1000.0)
        while len(batch) < max(1, batch_max):
            remaining = deadline - monotonic()
            if remaining <= 0:
                break
            try:
                queued = write_queue.get(timeout=remaining)
                if queued is write_stop:
                    run_write_batch(batch)
                    return
                batch.append(cast(_WriteTask, queued))
            except queue.Empty:
                break
        run_write_batch(batch)


def _ensure_write_worker(
    *,
    write_queue,
    write_thread,
    write_worker_lock,
    write_queue_max: int,
    worker_target,
    thread_name: str = "sobs-db-writer",
    queue_factory=queue.Queue,
    thread_factory=threading.Thread,
):
    if write_queue is not None and write_thread is not None and write_thread.is_alive():
        return write_queue, write_thread
    with write_worker_lock:
        next_queue = write_queue
        if next_queue is None:
            next_queue = queue_factory(maxsize=max(1, write_queue_max))
        next_thread = write_thread
        if next_thread is None or not next_thread.is_alive():
            next_thread = thread_factory(target=worker_target, name=thread_name, daemon=True)
            next_thread.start()
        return next_queue, next_thread


def _queue_write(
    op: Callable[[Any], None],
    *,
    ensure_write_worker,
    get_write_queue,
    wait: bool = False,
    write_task_cls=_WriteTask,
    event_factory=threading.Event,
    write_queue_full_error_cls=RuntimeError,
) -> None:
    ensure_write_worker()
    done = event_factory() if wait else None
    task = write_task_cls(op=op, done=done)
    write_queue = get_write_queue()
    assert write_queue is not None
    try:
        write_queue.put(task, timeout=1)
    except queue.Full as exc:
        raise write_queue_full_error_cls("write queue is full") from exc
    if done is not None:
        done.wait(timeout=15)
        if task.error is not None:
            raise task.error


def _write_queue_depth(write_queue) -> int:
    return write_queue.qsize() if write_queue is not None else 0


def _shutdown_write_worker(*, write_queue, write_thread, write_worker_lock, write_stop, join_timeout: int = 5):
    thread_to_join = None
    with write_worker_lock:
        if write_queue is not None and write_thread is not None and write_thread.is_alive():
            try:
                write_queue.put(write_stop, timeout=1)
            except queue.Full:
                pass
            thread_to_join = write_thread

    if thread_to_join is not None:
        thread_to_join.join(timeout=join_timeout)

    return None, None
