import io
import logging
import os
from typing import Any

import boto3
import torch
from PIL import Image
from torchvision import transforms
from transformers import AutoModelForImageSegmentation


LOGGER = logging.getLogger()
LOGGER.setLevel(logging.INFO)

MODEL_DIR = os.environ.get("LUCIDA_MODEL_DIR", "/opt/models/lucida")
MAX_FILE_BYTES = int(os.environ.get("MAX_FILE_BYTES", str(10 * 1024 * 1024)))
MAX_PIXELS = int(os.environ.get("MAX_PIXELS", "25000000"))

S3 = boto3.client("s3")
MODEL = AutoModelForImageSegmentation.from_pretrained(
    MODEL_DIR,
    trust_remote_code=True,
    local_files_only=True,
    dtype=torch.float32,
)
MODEL.eval()

PREPROCESS = transforms.Compose(
    [
        transforms.Resize((1024, 1024)),
        transforms.ToTensor(),
        transforms.Normalize(
            [0.485, 0.456, 0.406],
            [0.229, 0.224, 0.225],
        ),
    ]
)


def _required_string(event: dict[str, Any], name: str) -> str:
    value = event.get(name)
    if not isinstance(value, str) or not value:
        raise ValueError(f"{name} must be a non-empty string")
    return value


def _load_input(bucket: str, key: str) -> Image.Image:
    response = S3.get_object(Bucket=bucket, Key=key)
    content_length = response.get("ContentLength", 0)
    if content_length <= 0 or content_length > MAX_FILE_BYTES:
        raise ValueError("input image size is outside the allowed range")

    data = response["Body"].read(MAX_FILE_BYTES + 1)
    if len(data) > MAX_FILE_BYTES:
        raise ValueError("input image is too large")

    image = Image.open(io.BytesIO(data))
    if image.format not in {"JPEG", "PNG"}:
        raise ValueError("input must be a JPEG or PNG image")

    width, height = image.size
    if width <= 0 or height <= 0 or width * height > MAX_PIXELS:
        raise ValueError("input image dimensions are outside the allowed range")

    image.load()
    return image.convert("RGB")


def _remove_background(image: Image.Image) -> bytes:
    tensor = PREPROCESS(image).unsqueeze(0)

    with torch.inference_mode():
        prediction = MODEL(tensor)[-1].sigmoid().cpu()

    alpha = transforms.functional.resize(
        prediction[0],
        image.size[::-1],
        antialias=True,
    )[0]
    alpha_image = transforms.ToPILImage()(alpha)

    output = image.copy()
    output.putalpha(alpha_image)

    buffer = io.BytesIO()
    output.save(buffer, format="PNG")
    return buffer.getvalue()


def lambda_handler(event: dict[str, Any], context: Any) -> dict[str, Any]:
    input_bucket = _required_string(event, "input_bucket")
    input_key = _required_string(event, "input_key")
    output_bucket = _required_string(event, "output_bucket")
    output_key = _required_string(event, "output_key")

    LOGGER.info(
        "processing image request_id=%s input_bucket=%s output_bucket=%s",
        getattr(context, "aws_request_id", "unknown"),
        input_bucket,
        output_bucket,
    )

    image = _load_input(input_bucket, input_key)
    output_data = _remove_background(image)

    S3.put_object(
        Bucket=output_bucket,
        Key=output_key,
        Body=output_data,
        ContentType="image/png",
    )

    return {
        "output_bucket": output_bucket,
        "output_key": output_key,
        "content_type": "image/png",
        "size": len(output_data),
    }
