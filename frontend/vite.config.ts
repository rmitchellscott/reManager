import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import { createHash } from 'crypto'
import path from 'path'

// Build only: the dev server's inline React Refresh preamble and HMR websocket
// would be blocked by this policy.
function contentSecurityPolicy(): Plugin {
  return {
    name: 'content-security-policy',
    apply: 'build',
    transformIndexHtml: {
      order: 'post',
      handler(html) {
        const inlineScriptHashes = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)]
          .map(([, body]) => `'sha256-${createHash('sha256').update(body).digest('base64')}'`)
        const policy = [
          "default-src 'self'",
          `script-src 'self' ${inlineScriptHashes.join(' ')}`,
          "style-src 'self' 'unsafe-inline'",
          "img-src 'self' data: blob: https: http:",
          "font-src 'self' data:",
          "worker-src 'self' blob:",
          "connect-src 'self'",
          "frame-src 'none'",
          "object-src 'none'",
          "base-uri 'none'",
          "form-action 'none'",
        ].join('; ')
        return {
          html,
          tags: [{ tag: 'meta', attrs: { 'http-equiv': 'Content-Security-Policy', content: policy }, injectTo: 'head-prepend' }],
        }
      },
    },
  }
}

// Monaco is bundled and handed to the loader, so the loader's default CDN path
// is never used. Blanking it keeps the built app from naming a remote script.
function removeMonacoLoaderCdnPath(): Plugin {
  return {
    name: 'remove-monaco-loader-cdn-path',
    transform(code, id) {
      if (!id.includes('@monaco-editor/loader')) return null
      return code.replace(/https:\/\/cdn\.jsdelivr\.net\/npm\/monaco-editor@[^'"]*/g, '')
    },
  }
}

export default defineConfig({
  plugins: [react(), contentSecurityPolicy(), removeMonacoLoaderCdnPath()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@bindings': path.resolve(__dirname, './bindings'),
    },
  },
  server: {
    host: true,
  },
})
