# -*- coding: utf-8 -*-
"""Vendor-neutral TTS / caption / MP4 stand-in (DEV-098, DEV-049, DEV-100, E97).

The fake provider is for automatic tests only. Real voices, term readings,
regions and licences stay with U5; a production renderer stays with U6.
Disabled unless MATH_MEDIA_STANDIN is 1/on/true/yes (dev and test only).
"""
from __future__ import annotations

import os
import shutil
import struct
import subprocess
import tempfile

from .errors import MathError
from . import lesson as L

PROVIDER = "fake-standin"
AUDIO_MPEG = "audio/mpeg"
VIDEO_MP4 = "video/mp4"
VTT = "text/vtt; charset=utf-8"

# One silent MPEG-1 Layer III frame (32 kbps, 44.1 kHz, mono). Not a real voice.
_SILENT_MP3_FRAME = bytes.fromhex(
    "ff fb 90 64 00 00 00 00 00 00 00 00 00 00 00 00"
    "00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00"
    "00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00"
    "00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00"
    "00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00"
    "00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00"
)


def standin_enabled():
    """Stand-in media is off unless explicitly enabled.

    Production must leave MATH_MEDIA_STANDIN unset or set it to 0/off.
    Dev, CI and local tests set 1/on/true/yes. Silent MP3, minimal MP4 and
    fake OCR drafts must never ship to students or be published as real media.
    """
    v = (os.environ.get("MATH_MEDIA_STANDIN") or "").strip().lower()
    return v in ("1", "on", "true", "yes")


def _clamp(n, lo, hi):
    return lo if n < lo else hi if n > hi else n


def cue_timeline(lesson, speed=1.0, segment=None):
    """Build (start_ms, end_ms, text, beat_id, reveals_answer) from lesson cues.

    Timing follows edu.cues/1 est_ms, scaled by playback speed (DEV-097).
    """
    speed = _clamp(float(speed or 1.0), 0.5, 2.0)
    out = []
    t = 0
    for b in lesson.get("beats") or []:
        if segment and b.get("id") != segment:
            continue
        cues = b.get("cues") or []
        if not cues:
            text = b.get("say") or ""
            dur = max(800, int(len(text) * 80 / speed))
            out.append((t, t + dur, text, b.get("id"), bool(b.get("reveals_answer"))))
            t += dur
            continue
        for c in cues:
            dur = max(200, int((c.get("est_ms") or 800) / speed))
            out.append((t, t + dur, c.get("text") or "", b.get("id"), bool(b.get("reveals_answer"))))
            t += dur
    return out


def webvtt(cues, *, student=False, markers=None):
    """Cue-aligned VTT. The student track drops answer-revealing cues and any
    cue whose text contains an answer marker (server-side leak guard)."""
    markers = [m for m in (markers or []) if m]
    lines = ["WEBVTT", ""]
    n = 0
    for start, end, text, _beat, reveals in cues:
        if student and reveals:
            continue
        if student and text and any(m in text for m in markers):
            continue
        if not text:
            continue
        n += 1
        lines.append(str(n))
        lines.append("%s --> %s" % (_ts(start), _ts(end)))
        lines.append(text)
        lines.append("")
    return "\n".join(lines).encode("utf-8")


def _ts(ms):
    if ms < 0:
        ms = 0
    h, rem = divmod(ms, 3600000)
    m, rem = divmod(rem, 60000)
    s, milli = divmod(rem, 1000)
    return "%02d:%02d:%02d.%03d" % (h, m, s, milli)


def silent_mp3(duration_ms, volume=1.0):
    """A mute MP3 long enough for the cue timeline. Volume is recorded only
    (the stand-in is silence); it is still validated and returned to the caller."""
    _ = _clamp(float(volume if volume is not None else 1.0), 0.0, 1.0)
    # Each frame is 24 ms at 44.1 kHz / 1152 samples.
    n = max(1, int((max(duration_ms, 200) + 23) / 24))
    return b"".join([_SILENT_MP3_FRAME] * n)


def minimal_mp4(duration_ms=1000):
    """A tiny but well-formed MP4 (ftyp + moov/mvhd) when ffmpeg is absent."""
    # ftyp
    ftyp = _box(b"ftyp", b"isom" + struct.pack(">I", 512) + b"isomiso2mp41")
    # mvhd: version 0, 12-byte matrix skipped to a short header with timescale 1000.
    timescale, dur = 1000, max(int(duration_ms), 1)
    mvhd_body = (
        b"\x00" * 4 +  # version/flags
        b"\x00" * 8 +  # ctime/mtime
        struct.pack(">I", timescale) +
        struct.pack(">I", dur) +
        struct.pack(">I", 0x00010000) +  # rate
        struct.pack(">H", 0x0100) +  # volume
        b"\x00" * 10 +
        struct.pack(">9I", 0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000) +
        b"\x00" * 24 +
        struct.pack(">I", 2)  # next track id
    )
    moov = _box(b"moov", _box(b"mvhd", mvhd_body))
    mdat = _box(b"mdat", b"")
    return ftyp + moov + mdat


def _box(typ, payload):
    return struct.pack(">I", 8 + len(payload)) + typ + payload


def render_mp4(audio, duration_ms, work=None):
    """Prefer ffmpeg when present; otherwise a structural stand-in MP4."""
    ffmpeg = shutil.which("ffmpeg")
    if not ffmpeg:
        return minimal_mp4(duration_ms), "standin-box"
    tmp = work or tempfile.mkdtemp(prefix="edu-mp4-")
    try:
        wav = os.path.join(tmp, "a.mp3")
        out = os.path.join(tmp, "out.mp4")
        with open(wav, "wb") as f:
            f.write(audio or silent_mp3(duration_ms))
        dur = max(duration_ms / 1000.0, 0.2)
        cmd = [
            ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
            "-f", "lavfi", "-i", "color=c=black:s=320x240:d=%.3f" % dur,
            "-i", wav,
            "-c:v", "mpeg4", "-c:a", "aac", "-shortest",
            "-movflags", "+faststart", out,
        ]
        r = subprocess.run(cmd, capture_output=True, timeout=30)
        if r.returncode != 0 or not os.path.isfile(out) or os.path.getsize(out) < 16:
            return minimal_mp4(duration_ms), "standin-box"
        with open(out, "rb") as f:
            return f.read(), "ffmpeg"
    except (OSError, subprocess.SubprocessError):
        return minimal_mp4(duration_ms), "standin-box"
    finally:
        if work is None:
            shutil.rmtree(tmp, ignore_errors=True)


def project_student_captions(lesson, cues, reveal="released"):
    markers = L.answer_markers(lesson) if reveal != "released" else []
    # Even when released, the caption track itself must not embed the check key.
    if reveal == "released":
        k = lesson.get("check_answer_key") or {}
        markers = [m for m in (k.get("plain", ""), k.get("exact", "")) if m]
    data = webvtt(cues, student=True, markers=markers)
    # Fail closed if a marker still slipped through.
    text = data.decode("utf-8")
    leaked = [m for m in (L.answer_markers(lesson) if reveal != "released" else markers) if m and m in text]
    if leaked:
        raise MathError("VALIDATION_FAILED", "student captions would leak answer content", {"markers": len(leaked)})
    return data


def render(lesson, media="tts", *, speed=1.0, volume=1.0, segment=None, reveal="released"):
    """Produce stand-in audio/captions (and optional MP4) for one lesson."""
    if not standin_enabled():
        raise MathError("PROVIDER_UNAVAILABLE", "no TTS/MP4 provider is configured for this service")
    if media not in ("tts", "mp4"):
        raise MathError("VALIDATION_FAILED", "media must be tts or mp4")
    if not lesson or not (lesson.get("beats") or lesson.get("schema_version")):
        raise MathError("VALIDATION_FAILED", "lesson artifact is not a lesson document")
    speed = _clamp(float(speed or 1.0), 0.5, 2.0)
    volume = _clamp(float(volume or 1.0), 0.0, 1.0)
    cues = cue_timeline(lesson, speed=speed, segment=segment)
    if not cues:
        raise MathError("VALIDATION_FAILED", "no narration to render")
    duration = cues[-1][1]
    chars = sum(len(c[2]) for c in cues)
    audio = silent_mp3(duration, volume=volume)
    teacher_vtt = webvtt(cues, student=False)
    student_vtt = project_student_captions(lesson, cues, reveal=reveal)
    parts = {
        "provider": PROVIDER,
        "media": media,
        "speed": speed,
        "volume": volume,
        "segment": segment or "all",
        "duration_ms": duration,
        "characters": chars,
        "audio": audio,
        "teacher_vtt": teacher_vtt,
        "student_vtt": student_vtt,
    }
    if media == "mp4":
        video, how = render_mp4(audio, duration)
        parts["video"] = video
        parts["video_encoder"] = how
    return parts
