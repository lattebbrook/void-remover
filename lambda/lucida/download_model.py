import os

from huggingface_hub import snapshot_download


snapshot_download(
    repo_id="egeorcun/lucida",
    revision=os.environ["LUCIDA_MODEL_REVISION"],
    local_dir=os.environ["LUCIDA_MODEL_DIR"],
)
