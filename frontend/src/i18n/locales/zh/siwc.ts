export default {
  title: '添加 ChatGPT SIWC 账号',
  description: '使用独立的 ChatGPT 用量共享授权。是否可用取决于该账号获得的权限和模型列表。',
  reauthTitle: '重新授权 ChatGPT SIWC',
  reauthHelp: '沿用原账号的应用身份与服务端代理，只允许原用户重新授权；保留分组、倍率、名称和模型映射。账号若已被停用，授权后请检查状态并手动恢复。',
  name: '账号名称',
  proxy: '服务端代理',
  direct: '直接连接',
  groups: '绑定 OpenAI 分组',
  unbound: '不选择分组时仅保存账号，不自动加入客户分组。',
  concurrency: '并发数',
  start: '生成授权链接',
  open: '打开 ChatGPT 授权页',
  callback: '完整回调地址',
  callbackHelp: '完成授权后，浏览器可能提示 127.0.0.1 无法连接。复制地址栏中的完整回调地址粘贴到下方，10 分钟内完成。浏览器登录使用浏览器自身网络，服务端代理只用于换取凭证和模型请求。',
  save: '验证授权并保存账号',
  restart: '重新授权',
  failed: '授权失败，请检查回调地址或重新授权。'
}
