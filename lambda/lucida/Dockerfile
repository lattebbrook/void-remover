FROM public.ecr.aws/lambda/python:3.12

# Versions mirror Lucida's uv.lock at the pinned upstream revision.
COPY requirements.txt ${LAMBDA_TASK_ROOT}/requirements.txt
RUN pip install --no-cache-dir \
        --index-url https://download.pytorch.org/whl/cpu \
        torch==2.13.0 \
        torchvision==0.28.0 \
    && pip install --no-cache-dir -r ${LAMBDA_TASK_ROOT}/requirements.txt

ARG LUCIDA_MODEL_REVISION=a34eeda32a7f43b487e7a30532153f47946512fa
ENV LUCIDA_MODEL_DIR=/opt/models/lucida \
    HF_HUB_OFFLINE=1 \
    TRANSFORMERS_OFFLINE=1 \
    HF_HOME=/tmp/huggingface \
    XDG_CACHE_HOME=/tmp/cache

COPY download_model.py ${LAMBDA_TASK_ROOT}/download_model.py
RUN LUCIDA_MODEL_REVISION=${LUCIDA_MODEL_REVISION} \
    HF_HUB_OFFLINE=0 \
    TRANSFORMERS_OFFLINE=0 \
    python ${LAMBDA_TASK_ROOT}/download_model.py \
    && rm ${LAMBDA_TASK_ROOT}/download_model.py

COPY handler.py ${LAMBDA_TASK_ROOT}/handler.py

CMD ["handler.lambda_handler"]
