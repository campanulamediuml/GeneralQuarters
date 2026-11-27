#!/bin/sh
# 二进制里不再内嵌 content：容器第一次挂上空的卷时，从镜像里的 /app/content 复制一份到数据目录。
# 卷里已经有 content 就完全不动它，之后换剧本/换图直接往 /data/content 里放即可。
set -eu
: "${GQ_DATA_DIR:=/data}"
if [ ! -d "$GQ_DATA_DIR/content" ]; then
  mkdir -p "$GQ_DATA_DIR"
  cp -R /app/content "$GQ_DATA_DIR/content"
fi
exec /usr/local/bin/general-quarters "$@"
