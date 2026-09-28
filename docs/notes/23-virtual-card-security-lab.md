# 23. 虚拟卡安全实验室

## 已完成

- 新增 `tools/virtual_lab/virtual_card_lab.py`，提供完全离线的合成虚拟卡。
- 使用 PBKDF2 派生教学秘密，并用一次性 challenge + HMAC 模拟动态认证。
- 增加固定上限为 64 次的候选口令审计，仅允许对虚拟卡对象执行。
- 增加静态快照和静态克隆模型；快照不包含动态认证秘密。
- 增加 UID 冲突、静态指纹、计数器复用和动态认证失败检测。
- 增加 Python 标准库回归测试和运行说明。

## 安全边界

- 不连接真实硬件，不调用 libnfc，不读取真实卡片，不接收真实 dump，不访问网络。
- 不实现认证绕过、无限爆破、未知密钥恢复或实体卡克隆。
- 实验协议为教学模型，不应被当作真实卡片协议或生产密码方案。

## 验证

- `python3 -m unittest -v tools/virtual_lab/test_virtual_card_lab.py`：待运行。
- `python3 tools/virtual_lab/virtual_card_lab.py --demo`：待运行。
- Go/Wails 构建：当前沙箱没有 Go 工具链，未运行。

## 问题与阻塞

无。
