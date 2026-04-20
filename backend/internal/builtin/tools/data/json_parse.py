# @builtin
# @version 1.0
# @category data
# @display_name JSON 解析
# @description 解析 JSON 字符串，支持 JSONPath 提取

def json_parse(text: str, path: str = "") -> dict:
    """解析 JSON 字符串，可选 JSONPath 提取"""
    import json
    data = json.loads(text)
    if not path:
        return {"result": data}
    # Simple dot-notation path extraction
    parts = path.strip(".").split(".")
    current = data
    for part in parts:
        if isinstance(current, dict):
            current = current.get(part)
        elif isinstance(current, list) and part.isdigit():
            current = current[int(part)]
        else:
            return {"result": None, "error": f"Path '{path}' not found"}
    return {"result": current}
