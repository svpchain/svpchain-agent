import MarkdownIt from 'markdown-it'

// html: false keeps raw HTML in model output escaped — the assistant's text is
// untrusted input, so markup only ever comes from markdown-it itself.
const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
})

// Links open in the system browser (see the click interceptor in AgentView);
// target/rel here are defense in depth for the webview.
const defaultLinkOpen =
  md.renderer.rules.link_open ||
  ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options))
md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
  tokens[idx].attrSet('target', '_blank')
  tokens[idx].attrSet('rel', 'noopener noreferrer')
  return defaultLinkOpen(tokens, idx, options, env, self)
}

export function renderMarkdown(text: string): string {
  return md.render(text)
}
