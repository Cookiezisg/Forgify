# @builtin
# @version 1.0
# @category system
# @display_name 延时等待
# @description 等待指定秒数

def delay(seconds: float = 1.0) -> dict:
    """等待指定秒数后返回"""
    import time
    time.sleep(seconds)
    return {"waited": seconds}
