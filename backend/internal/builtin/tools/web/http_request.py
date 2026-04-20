# @builtin
# @version 1.0
# @category web
# @display_name HTTP 请求
# @description 发送 HTTP 请求，返回响应内容

def http_request(url: str, method: str = "GET", headers: dict = {}, body: str = "", timeout: int = 30) -> dict:
    """发送 HTTP 请求"""
    import requests
    resp = requests.request(method, url, headers=headers, data=body, timeout=timeout)
    return {
        "status_code": resp.status_code,
        "body": resp.text[:10000],
        "headers": dict(resp.headers),
    }
