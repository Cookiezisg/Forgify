# @builtin
# @version 1.0
# @category file
# @display_name 读取文本文件
# @description 读取本地文本文件内容，支持指定编码

def file_read(file_path: str, encoding: str = "utf-8", max_lines: int = 1000) -> dict:
    """读取本地文本文件内容"""
    with open(file_path, "r", encoding=encoding) as f:
        lines = []
        for i, line in enumerate(f):
            if i >= max_lines:
                return {"content": "".join(lines), "truncated": True, "lines_read": max_lines}
            lines.append(line)
    return {"content": "".join(lines), "truncated": False, "lines_read": len(lines)}
