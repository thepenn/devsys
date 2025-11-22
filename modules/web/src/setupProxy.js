const { createProxyMiddleware } = require('http-proxy-middleware');

module.exports = function (app) {
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
