# -*- coding: utf-8 -*-
"""Stand-in TTS, captions, MP4 and OCR (E97). Auto-tested only; not a real vendor."""
import base64
import json
import os
import struct
import tempfile
import unittest
import zlib

from mathsvc import lesson as L
from mathsvc import standin_media as M
from mathsvc import standin_ocr as O
from mathsvc.errors import MathError
from mathsvc.jobs import Store
from mathsvc.server import App
from tests.test_families import GOLD, run_all


def png():
    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
    raw = b"\x00\xff\x00\x00"
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")


class StandinMedia(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        prior, lp = run_all(GOLD["SOLID-ANGLE-01"][0][0])
        cls.lesson = lp["compose_lesson"]["lesson"]
        cls.sol = prior["solve"]["solution"]

    def test_captions_align_and_student_track_does_not_leak(self):
        r = M.render(self.lesson, "tts", speed=1.0, volume=0.8)
        self.assertEqual(r["provider"], "fake-standin")
        self.assertGreater(r["characters"], 0)
        self.assertGreater(r["duration_ms"], 0)
        self.assertGreater(len(r["audio"]), 16)
        teacher = r["teacher_vtt"].decode("utf-8")
        student = r["student_vtt"].decode("utf-8")
        self.assertTrue(teacher.startswith("WEBVTT"))
        self.assertTrue(student.startswith("WEBVTT"))
        self.assertIn("-->", teacher)
        markers = L.answer_markers(self.lesson)
        leaked = [m for m in markers if m and m in student]
        self.assertEqual(leaked, [], student)
        # Teacher track may mention the worked result; the student track must not.
        self.assertGreater(len(teacher), len(student) - 32)

    def test_speed_scales_duration_and_partial_segment_is_shorter(self):
        full = M.render(self.lesson, "tts", speed=1.0)
        fast = M.render(self.lesson, "tts", speed=2.0)
        self.assertLess(fast["duration_ms"], full["duration_ms"])
        beat = self.lesson["beats"][0]["id"]
        part = M.render(self.lesson, "tts", segment=beat)
        self.assertEqual(part["segment"], beat)
        self.assertLess(part["duration_ms"], full["duration_ms"])
        self.assertLess(part["characters"], full["characters"])

    def test_mp4_is_an_iso_box(self):
        r = M.render(self.lesson, "mp4")
        self.assertIn(r["video_encoder"], ("ffmpeg", "standin-box"))
        self.assertGreater(len(r["video"]), 16)
        self.assertEqual(r["video"][4:8], b"ftyp")

    def test_disabled_standin_is_unavailable(self):
        old = os.environ.get("MATH_MEDIA_STANDIN")
        os.environ["MATH_MEDIA_STANDIN"] = "off"
        try:
            with self.assertRaises(MathError) as cm:
                M.render(self.lesson, "tts")
            self.assertEqual(cm.exception.code, "PROVIDER_UNAVAILABLE")
            with self.assertRaises(MathError) as cm:
                O.recognize(png())
            self.assertEqual(cm.exception.code, "PROVIDER_UNAVAILABLE")
        finally:
            if old is None:
                os.environ.pop("MATH_MEDIA_STANDIN", None)
            else:
                os.environ["MATH_MEDIA_STANDIN"] = old

    def test_ocr_draft_always_needs_confirmation(self):
        with self.assertRaises(MathError):
            O.recognize(b"")
        d = O.recognize(png())
        self.assertEqual(d["state"], "needs_confirmation")
        self.assertEqual(d["family_id"], d["structured"]["family_id"])
        self.assertEqual(d["provider"], "fake-standin")
        self.assertIn("intake", d["ai_parts"])

    def test_app_render_and_ocr_round_trip(self):
        d = tempfile.mkdtemp()
        app = App(b"k" * 32, d)
        art = app.store.put_artifact("t1", "mj_" + "a" * 32, "lesson", "teacher", "application/json",
                                     json.dumps(self.lesson, ensure_ascii=False).encode("utf-8"))
        out = app.media_render("t1", {"lesson_artifact_id": art["artifact_id"], "media": "tts", "speed": 1.0, "volume": 1.0})
        self.assertEqual(out["status"], "succeeded")
        self.assertIn("narration", out["ai_parts"])
        self.assertIn("audio", out["ai_parts"])
        kinds = {(a["kind"], a["audience"]) for a in out["artifacts"]}
        self.assertTrue({("audio", "teacher"), ("audio", "student"), ("captions", "student"), ("lesson", "teacher")} <= kinds)
        # Updated lesson records AI parts so provenance is no longer "no generative AI".
        les = [a for a in out["artifacts"] if a["kind"] == "lesson"][0]
        _, data = app.store.get_artifact("t1", les["artifact_id"])
        meta = json.loads(data)["meta"]
        self.assertEqual(L.provenance(meta)["kind"], "model_assisted")
        self.assertIn("配音", L.provenance(meta)["label"])
        # Missing artifact still errors (existing MCP contract).
        with self.assertRaises(MathError):
            app.media_render("t1", {"lesson_artifact_id": "ma_" + "0" * 32, "media": "tts"})
        ocr = app.ocr_intake({"image_b64": base64.b64encode(png()).decode("ascii"), "mime": "image/png"})
        self.assertEqual(ocr["state"], "needs_confirmation")


if __name__ == "__main__":
    unittest.main(verbosity=2)
