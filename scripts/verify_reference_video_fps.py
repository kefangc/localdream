"""Run the installed VHS_LoadVideo on real clips without queueing any prompt.

Usage (ComfyUI's Python):
  python scripts/verify_reference_video_fps.py --comfy-root <root> \
    --sources <clips-dir> --graphs <compiled-audit.json> --output <report.json>

Decoding runs on CPU. No model, sampler or H3 VAE is executed. H3 retained frame
counts are calculated from the installed ReferenceToVideo code, not measured
from GPU inference. --generation-frames defaults to the 5-second H3 grid (124).
"""

import argparse
import gc
import json
import shutil
import sys
import tempfile
from pathlib import Path
from types import SimpleNamespace


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--comfy-root", type=Path, required=True)
    parser.add_argument("--sources", type=Path, required=True)
    parser.add_argument("--graphs", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--generation-frames", type=int, default=124)
    options = parser.parse_args()
    sys.path.insert(0, str(options.comfy_root))
    sys.path.insert(0, str(options.comfy_root / "custom_nodes" / "comfyui-videohelpersuite"))
    sys.argv = [sys.argv[0], "--cpu"]
    from comfy.cli_args import args
    args.cpu = True
    import folder_paths
    import server
    # VHS utilities capture this queue on import. Decoding never uses it; there
    # is deliberately no server or worker to submit a GPU task to.
    server.PromptServer.instance = SimpleNamespace(prompt_queue=None, number=0)
    from videohelpersuite.load_video_nodes import LoadVideoUpload

    temporary_input = tempfile.TemporaryDirectory(prefix="reference-fps-audit-")
    folder_paths.set_input_directory(temporary_input.name)
    graphs = json.loads(options.graphs.read_text(encoding="utf-8"))
    results = []
    for key in ("normal_7fps", "normal_24fps"):
        graph = graphs[key]["prompt"]
        loaders = [node for node in graph.values()
                   if node["class_type"] == "VHS_LoadVideo"
                   and node.get("_meta", {}).get("title") == "dynamic reference video"]
        for node in loaders:
            inputs = node["inputs"]
            source_name = inputs["video"].removeprefix("localdream_")
            shutil.copy2(options.sources / source_name, Path(temporary_input.name) / inputs["video"])
            images, count, audio, info = LoadVideoUpload().load_video(**inputs)
            assert count == len(images) == info["loaded_frame_count"]
            assert abs(info["loaded_fps"] - inputs["force_rate"]) < 1e-6
            capped = min(count, options.generation_frames)
            retained = None if capped < 5 else capped - ((capped - 5) % 17)
            row = {"graph": key, "file": inputs["video"], "force_rate": inputs["force_rate"],
                   "tensor_shape": list(images.shape), "tensor_device": str(images.device),
                   "video_info": info, "h3_retained_frames_calculated": retained,
                   "h3_generation_frame_cap": options.generation_frames,
                   "h3_would_reject_under_5_frames": capped < 5,
                   "h3_interpreted_seconds_calculated": retained / 24 if retained else None}
            results.append(row)
            print(json.dumps(row, ensure_ascii=False), flush=True)
            del images, audio
            gc.collect()
    report = {"no_prompt_submitted": True, "no_model_or_sampler_executed": True,
              "loader": "installed VHS LoadVideoUpload (VHS_LoadVideo), CPU",
              "h3_trim_evidence": str(options.comfy_root / "comfy_extras" / "nodes_minimax_h3.py"),
              "h3_counts_are_calculated_not_encoded": True, "results": results}
    options.output.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    temporary_input.cleanup()


if __name__ == "__main__":
    main()
