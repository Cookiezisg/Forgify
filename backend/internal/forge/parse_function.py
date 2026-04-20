"""Parse Python function metadata using AST. Invoked via subprocess, reads code from stdin, outputs JSON."""
import ast
import json
import re
import sys

STDLIB = {
    "os","sys","json","re","datetime","time","math","random","collections","itertools",
    "functools","pathlib","io","typing","dataclasses","enum","abc","copy","hashlib",
    "hmac","base64","urllib","http","email","smtplib","csv","sqlite3","subprocess",
    "threading","multiprocessing","logging","unittest","contextlib","string","struct",
    "textwrap","unicodedata","decimal","fractions","statistics","pprint","tempfile",
    "shutil","glob","configparser","argparse","warnings","traceback","inspect","types",
    "weakref","array","heapq","bisect","queue","socket","ssl","html","xml","uuid",
    "platform","locale","pickle","operator","asyncio","concurrent","secrets","zipfile",
    "tarfile","gzip","ast",
}

def base_type(full_type: str) -> str:
    """Extract base type: 'list[int]' → 'list', 'Optional[str]' → 'str', 'dict' → 'dict'"""
    # Optional[X] → X
    m = re.match(r'Optional\[(.+)\]', full_type)
    if m:
        return base_type(m.group(1))
    # list[X] → list, dict[X,Y] → dict
    m = re.match(r'(\w+)\[', full_type)
    if m:
        return m.group(1)
    return full_type

def parse_meta(code: str) -> dict:
    """Extract @display_name, @description, @category from comment lines."""
    meta = {}
    for line in code.split('\n'):
        line = line.strip()
        if not line.startswith('#'):
            if line:
                break
            continue
        comment = line.lstrip('#').strip()
        if comment.startswith('@display_name '):
            meta['display_name'] = comment[len('@display_name '):]
        elif comment.startswith('@description '):
            meta['description'] = comment[len('@description '):]
        elif comment.startswith('@category '):
            meta['category'] = comment[len('@category '):]
        elif comment.startswith('@builtin'):
            meta['builtin'] = True
        elif comment.startswith('@version '):
            meta['version'] = comment[len('@version '):]
        elif comment.startswith('@requires_key '):
            meta['requires_key'] = comment[len('@requires_key '):]
    return meta

def parse_function(code: str) -> dict:
    try:
        tree = ast.parse(code)
    except SyntaxError as e:
        return {"error": f"SyntaxError: {e.msg} (line {e.lineno})"}

    # Find first top-level function
    func = None
    for node in ast.iter_child_nodes(tree):
        if isinstance(node, ast.FunctionDef):
            func = node
            break

    if not func:
        return {"error": "No function definition found"}

    # Extract parameters
    params = []
    args = func.args
    num_defaults = len(args.defaults)
    num_args = len(args.args)

    for i, arg in enumerate(args.args):
        p = {"name": arg.arg, "required": True}
        if arg.annotation:
            full = ast.unparse(arg.annotation)
            p["type"] = full
            p["base_type"] = base_type(full)
        else:
            p["type"] = "Any"
            p["base_type"] = "str"

        # Check if has default
        default_idx = i - (num_args - num_defaults)
        if default_idx >= 0:
            p["required"] = False
            p["default"] = ast.unparse(args.defaults[default_idx])

        params.append(p)

    # Extract docstring
    docstring = ast.get_docstring(func) or ""

    # Extract imports
    imports = []
    seen = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                pkg = alias.name.split('.')[0]
                if pkg not in STDLIB and pkg not in seen:
                    imports.append(pkg)
                    seen.add(pkg)
        elif isinstance(node, ast.ImportFrom) and node.module:
            pkg = node.module.split('.')[0]
            if pkg not in STDLIB and pkg not in seen:
                imports.append(pkg)
                seen.add(pkg)

    # Extract @metadata
    meta = parse_meta(code)

    return {
        "function_name": func.name,
        "docstring": docstring,
        "parameters": params,
        "requirements": imports,
        "display_name": meta.get("display_name", ""),
        "description": meta.get("description", ""),
        "category": meta.get("category", "other"),
        "is_builtin": meta.get("builtin", False),
        "version": meta.get("version", "1.0"),
        "requires_key": meta.get("requires_key", ""),
    }

if __name__ == "__main__":
    code = sys.stdin.read()
    result = parse_function(code)
    print(json.dumps(result, ensure_ascii=False))
