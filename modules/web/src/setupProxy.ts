const { createProxyMiddleware } = require('http-proxy-middleware');

module.exports = function (app: { use: (...args: unknown[]) => void }) {
  app.use(
    ['/api', '/admin', '/sys'],
    createProxyMiddleware({
      target: 'http://localhost:8080',
      changeOrigin: true,
      secure: false,
      logLevel: 'warn'
    })
  );
};
