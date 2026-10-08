export default {
  title: 'Add ChatGPT SIWC account',
  description: 'Use a separate ChatGPT token-sharing grant. Availability depends on the granted scopes and account model catalog.',
  reauthTitle: 'Reauthorize ChatGPT SIWC',
  reauthHelp: 'Reuse the original application identity and server proxy. Only the original subject can authorize again. Groups, rates, name and model mappings are retained. If disabled, review and recover the account state after authorization.',
  name: 'Account name',
  proxy: 'Server proxy',
  direct: 'Direct connection',
  groups: 'OpenAI groups',
  unbound: 'Leaving groups empty saves the account without adding it to customer groups.',
  concurrency: 'Concurrency',
  start: 'Generate authorization link',
  open: 'Open ChatGPT authorization',
  callback: 'Complete callback URL',
  callbackHelp: 'After authorization, the browser may report that 127.0.0.1 is unavailable. Paste the complete address-bar URL below within 10 minutes. Browser login uses its own network; the server proxy applies to token exchange and model requests.',
  save: 'Verify and save account',
  restart: 'Restart authorization',
  failed: 'Authorization failed. Check the callback URL or authorize again.'
}
