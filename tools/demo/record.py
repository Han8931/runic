#!/usr/bin/env python3
"""Drive a terminal program in a pty and write an asciicast v2 recording.

Usage:
    record.py <steps-file> <output.cast> [-- command ...]

The command defaults to an interactive bash with a minimal prompt. Terminal
size comes from DEMO_COLS / DEMO_ROWS (default 120x32).

Steps file syntax (one directive per line, '#' starts a comment):

    delay <sec>       set the per-character delay used by `type` / `run`
    gap <sec>         set the delay between keys emitted by one `key` line
    wait <sec>        sleep, letting the program paint
    type <text>       type text character by character
    run <text>        `type <text>` followed by Enter
    key <name>...     send named keys in order (see KEYS below)
    raw <text>        send text with \\e \\n \\r \\t \\\\ escapes honoured
    marker <label>    record an asciicast marker (for `agg --select`)
"""

from __future__ import annotations

import codecs
import fcntl
import json
import os
import pty
import re
import shlex
import signal
import struct
import sys
import termios
import threading
import time

KEYS = {
    "enter": "\r",
    "cr": "\r",
    "tab": "\t",
    "stab": "\x1b[Z",
    "esc": "\x1b",
    "space": " ",
    "bs": "\x7f",
    "up": "\x1b[A",
    "down": "\x1b[B",
    "right": "\x1b[C",
    "left": "\x1b[D",
    "home": "\x1b[H",
    "end": "\x1b[F",
    "pgup": "\x1b[5~",
    "pgdn": "\x1b[6~",
}

ESCAPES = {"e": "\x1b", "n": "\n", "r": "\r", "t": "\t", "\\": "\\", "0": "\0"}

# A pty is not a terminal emulator: nothing answers the capability queries that
# TUI toolkits send at startup, so a program that waits for a reply stalls until
# its own timeout (runic queries OSC 11 for the background, Bubble Tea sends a
# cursor-position report). Answering them keeps the recording honest — the
# program sees the same handshake a real terminal would give it.
#
# The background answer is Dracula's #282a36, matching the `agg --theme` used
# for the GIF, so adaptive colors resolve to the palette actually rendered.
DEMO_BACKGROUND = "\x1b]11;rgb:2828/2a2a/3636\x07"

QUERY_REPLIES: list[tuple[re.Pattern, str]] = [
    (re.compile(r"\x1b\]11;\?(\x07|\x1b\\)"), DEMO_BACKGROUND),
    (re.compile(r"\x1b\]10;\?(\x07|\x1b\\)"), "\x1b]10;rgb:f8f8/f8f8/f2f2\x07"),
    (re.compile(r"\x1b\[6n"), "\x1b[1;1R"),
    (re.compile(r"\x1b\[\?6n"), "\x1b[?1;1R"),
    (re.compile(r"\x1b\[>0?c"), "\x1b[>0;276;0c"),
    (re.compile(r"\x1b\[[0-9]*c"), "\x1b[?62;22c"),
    (re.compile(r"\x1b\[>q"), "\x1bP>|demo-recorder\x1b\\"),
    (re.compile(r"\x1bP\+q[0-9a-fA-F;]*\x1b\\"), "\x1bP0+r\x1b\\"),
]


def key_bytes(name: str) -> str:
    """Translate one key name into the bytes a terminal would send."""
    if name in KEYS:
        return KEYS[name]
    if name.startswith("ctrl-") and len(name) == 6:
        ch = name[5].lower()
        if "a" <= ch <= "z":
            return chr(ord(ch) - 96)
    if len(name) == 1:
        return name
    raise ValueError(f"unknown key: {name!r}")


def unescape(text: str) -> str:
    out: list[str] = []
    i = 0
    while i < len(text):
        c = text[i]
        if c == "\\" and i + 1 < len(text):
            nxt = text[i + 1]
            if nxt in ESCAPES:
                out.append(ESCAPES[nxt])
                i += 2
                continue
        out.append(c)
        i += 1
    return "".join(out)


def set_winsize(fd: int, cols: int, rows: int) -> None:
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))


class Recorder:
    def __init__(self, command: list[str], cols: int, rows: int) -> None:
        self.cols = cols
        self.rows = rows
        self.events: list[list] = []
        self.lock = threading.Lock()
        self.alive = True
        self.end: float | None = None

        pid, fd = pty.fork()
        if pid == 0:
            env = dict(os.environ)
            env["TERM"] = env.get("TERM", "xterm-256color")
            env["COLUMNS"] = str(cols)
            env["LINES"] = str(rows)
            try:
                os.execvpe(command[0], command, env)
            finally:
                os._exit(127)

        self.pid = pid
        self.fd = fd
        set_winsize(fd, cols, rows)
        self.start = time.monotonic()
        self.reader = threading.Thread(target=self._read_loop, daemon=True)
        self.reader.start()

    def _read_loop(self) -> None:
        pending = ""
        # A read can land mid-character: box-drawing glyphs are three bytes, so
        # decoding each chunk on its own would punch U+FFFD holes in the frames.
        decoder = codecs.getincrementaldecoder("utf-8")("replace")
        while True:
            try:
                data = os.read(self.fd, 65536)
            except OSError:
                break
            if not data:
                break
            text = decoder.decode(data)
            if not text:
                continue
            with self.lock:
                self.events.append([time.monotonic() - self.start, "o", text])
            pending = self._answer_queries(pending + text)
        self.alive = False

    def _answer_queries(self, buf: str) -> str:
        """Reply to terminal capability queries; return the unconsumed tail."""
        while True:
            best: tuple[int, int, str] | None = None
            for pattern, reply in QUERY_REPLIES:
                m = pattern.search(buf)
                if m and (best is None or m.start() < best[0]):
                    best = (m.start(), m.end(), reply)
            if best is None:
                break
            try:
                os.write(self.fd, best[2].encode())
            except OSError:
                return ""
            buf = buf[best[1] :]
        # Keep only enough tail to catch a query split across two reads.
        return buf[-32:]

    def send(self, text: str) -> None:
        os.write(self.fd, text.encode())

    def marker(self, label: str) -> None:
        with self.lock:
            self.events.append([time.monotonic() - self.start, "m", label])

    def finish(self) -> None:
        """Mark the end of the scripted demo.

        Anything the program paints after this — a TUI restoring the primary
        screen as it is torn down, a shell's exit notice — is teardown, not
        demo, and would otherwise become the GIF's last (and longest) frame.
        """
        self.end = time.monotonic() - self.start

    def close(self, grace: float = 1.0) -> None:
        deadline = time.monotonic() + grace
        while self.alive and time.monotonic() < deadline:
            time.sleep(0.05)
        try:
            os.kill(self.pid, signal.SIGHUP)
        except ProcessLookupError:
            pass
        try:
            os.waitpid(self.pid, 0)
        except ChildProcessError:
            pass
        self.reader.join(timeout=1.0)
        try:
            os.close(self.fd)
        except OSError:
            pass

    def write_cast(self, path: str) -> float:
        header = {
            "version": 2,
            "width": self.cols,
            "height": self.rows,
            "timestamp": int(os.environ.get("DEMO_TIMESTAMP", "0")) or None,
            "env": {"SHELL": os.environ.get("SHELL", "/bin/bash"), "TERM": "xterm-256color"},
        }
        header = {k: v for k, v in header.items() if v is not None}
        with self.lock:
            events = sorted(self.events, key=lambda e: e[0])
        if self.end is not None:
            events = [e for e in events if e[0] <= self.end]
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(json.dumps(header) + "\n")
            for ev in events:
                fh.write(json.dumps(ev) + "\n")
        return events[-1][0] if events else 0.0


def play(rec: Recorder, steps: list[str]) -> None:
    char_delay = 0.05
    gap = 0.12

    for lineno, raw_line in enumerate(steps, 1):
        line = raw_line.rstrip("\n")
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue

        head, _, rest = stripped.partition(" ")
        rest = rest.strip()

        if head == "delay":
            char_delay = float(rest)
        elif head == "gap":
            gap = float(rest)
        elif head == "wait":
            time.sleep(float(rest))
        elif head == "marker":
            rec.marker(rest)
        elif head in ("type", "run"):
            for ch in rest:
                rec.send(ch)
                time.sleep(char_delay)
            if head == "run":
                time.sleep(0.15)
                rec.send("\r")
        elif head == "key":
            for name in rest.split():
                rec.send(key_bytes(name))
                time.sleep(gap)
        elif head == "raw":
            rec.send(unescape(rest))
        else:
            raise SystemExit(f"{lineno}: unknown directive {head!r}")

        if not rec.alive and head not in ("wait", "marker"):
            print(f"note: program exited before line {lineno}", file=sys.stderr)
            break


def main() -> None:
    argv = sys.argv[1:]
    command = ["bash", "--norc", "--noprofile", "-i"]
    if "--" in argv:
        idx = argv.index("--")
        command = argv[idx + 1 :]
        argv = argv[:idx]
    if len(argv) != 2:
        raise SystemExit(__doc__)

    steps_path, out_path = argv
    with open(steps_path, encoding="utf-8") as fh:
        steps = fh.readlines()

    cols = int(os.environ.get("DEMO_COLS", "120"))
    rows = int(os.environ.get("DEMO_ROWS", "32"))

    rec = Recorder(command, cols, rows)
    try:
        play(rec, steps)
    finally:
        rec.finish()
        rec.close()
        duration = rec.write_cast(out_path)
    print(f"wrote {out_path}: {duration:.1f}s, {len(rec.events)} events, {cols}x{rows}")
    print(f"command: {shlex.join(command)}")


if __name__ == "__main__":
    main()
