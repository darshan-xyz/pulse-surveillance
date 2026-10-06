import { defineConfig, Plugin } from "vite";
import react from "@vitejs/plugin-react-swc";
import path from "path";
import { createServer } from "./server";

// Development configuration with server plugin
// Note: Vite may still log "[vite] http proxy error:" for DNS/network failures (ENOTFOUND, ECONNRESET)
// These errors are expected when DNS fails and are automatically handled by client-side fallback.
// The proxy error handlers suppress these at the proxy level, and our plugin attempts to suppress them
// at the logger level. If you still see them, they're harmless - the client will automatically retry
// with a direct connection. To see all proxy logs for debugging, set VITE_DEBUG_PROXY=true.
export default defineConfig(({ mode }) => ({
  server: {
    host: "::",
    port: 8080,
    hmr: {
      overlay: false, // Disable error overlay to prevent DOM conflicts
      // Reduce HMR issues with better configuration
      protocol: 'ws',
      host: 'localhost',
    },
    proxy: {
      '/auth': {
        target: 'https://bujskrxtswtogpamnflh.supabase.co',
        changeOrigin: true,
        secure: true,
        timeout: 10000, // 10 second timeout for proxy requests
        proxyTimeout: 10000, // 10 second timeout for proxy responses
        rewrite: (path) => path.replace(/^\/auth/, '/auth'),
        configure: (proxy, options) => {
          // Suppress all proxy errors by default - they're handled by client-side fallback
          // Vite will still log "[vite] http proxy error:" but we prevent our own logging
          proxy.on('error', (err, req, res) => {
            // Completely suppress common network errors (ECONNRESET, ENOTFOUND)
            // These are expected when DNS fails and are handled by client-side fallback
            const isCommonNetworkError = 
              err.code === 'ECONNRESET' || 
              err.code === 'ENOTFOUND' ||
              err.message?.includes('getaddrinfo') ||
              err.message?.includes('ENOTFOUND');
            
            // Only log non-common errors if debug is explicitly enabled
            if (!isCommonNetworkError && process.env.VITE_DEBUG_PROXY === 'true') {
              console.error('Auth proxy error:', err.message || err);
            }
            // For common network errors, do nothing - let client handle fallback silently
          });
          // Only log errors and non-200 responses if debug enabled
          proxy.on('proxyRes', (proxyRes, req, res) => {
            if (proxyRes.statusCode >= 400 && process.env.VITE_DEBUG_PROXY === 'true') {
              console.warn(`Auth proxy error [${proxyRes.statusCode}]:`, req.method, req.url);
            }
          });
        }
      },
      '/rest': {
        target: 'https://bujskrxtswtogpamnflh.supabase.co',
        changeOrigin: true,
        secure: true,
        timeout: 10000, // 10 second timeout for proxy requests
        proxyTimeout: 10000, // 10 second timeout for proxy responses
        rewrite: (path) => path.replace(/^\/rest/, '/rest'),
        configure: (proxy, options) => {
          // Suppress all proxy errors by default - they're handled by client-side fallback
          // Vite will still log "[vite] http proxy error:" but we prevent our own logging
          proxy.on('error', (err, req, res) => {
            // Completely suppress common network errors (ECONNRESET, ENOTFOUND)
            // These are expected when DNS fails and are handled by client-side fallback
            const isCommonNetworkError = 
              err.code === 'ECONNRESET' || 
              err.code === 'ENOTFOUND' ||
              err.message?.includes('getaddrinfo') ||
              err.message?.includes('ENOTFOUND');
            
            // Only log non-common errors if debug is explicitly enabled
            if (!isCommonNetworkError && process.env.VITE_DEBUG_PROXY === 'true') {
              console.error('REST proxy error:', err.message || err);
            }
            // For common network errors, do nothing - let client handle fallback silently
          });
          // Only log errors and non-200 responses if debug enabled
          proxy.on('proxyRes', (proxyRes, req, res) => {
            if (proxyRes.statusCode >= 400 && process.env.VITE_DEBUG_PROXY === 'true') {
              console.warn(`REST proxy error [${proxyRes.statusCode}]:`, req.method, req.url);
            }
          });
        }
      }
    },
  },
  build: {
    outDir: "dist/spa",
  },
  plugins: [
    react(), 
    expressPlugin(),
    suppressProxyErrorsPlugin()
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./client"),
      "@shared": path.resolve(__dirname, "./shared"),
    },
  },
  // Improve development stability
  optimizeDeps: {
    include: ['react', 'react-dom'],
  },
}));

function expressPlugin(): Plugin {
  return {
    name: "express-plugin",
    apply: "serve", // Only apply during development (serve mode)
    configureServer(server) {
      const app = createServer();

      // Add Express app as middleware to Vite dev server
      server.middlewares.use(app);
    },
  };
}

// Plugin to suppress Vite's proxy DNS errors (ENOTFOUND) - these are handled by client-side fallback
// Note: Vite logs proxy errors directly via its internal logger, which we intercept here
function suppressProxyErrorsPlugin(): Plugin {
  return {
    name: "suppress-proxy-errors",
    apply: "serve",
    configureServer(server) {
      // Only suppress if debug is not enabled
      if (process.env.VITE_DEBUG_PROXY !== 'true') {
        // Intercept Vite's logger by wrapping the server's logger
        // Vite uses its logger to output proxy errors, so we intercept at that level
        const originalError = console.error.bind(console);
        const originalWarn = console.warn.bind(console);
        
        const shouldSuppress = (message: string): boolean => {
          if (!message) return false;
          const msg = typeof message === 'string' ? message : String(message);
          // Match Vite's proxy error format: "[vite] http proxy error: ... ENOTFOUND/getaddrinfo"
          return (
            (msg.includes('[vite] http proxy error:') || 
             msg.includes('http proxy error') ||
             msg.match(/\[vite\].*proxy.*error/i)) &&
            (msg.includes('ENOTFOUND') || 
             msg.includes('getaddrinfo') || 
             msg.includes('ECONNRESET') ||
             msg.includes('ETIMEDOUT') ||
             msg.includes('EAI_AGAIN'))
          );
        };
        
        // Override console methods - Vite's logger uses these
        // We intercept synchronously to catch messages early
        const wrappedError = (...args: any[]) => {
          const fullMessage = args.map(arg => {
            if (typeof arg === 'string') return arg;
            if (arg instanceof Error) return arg.message + ' ' + (arg.stack || '');
            try {
              return String(arg);
            } catch {
              return '';
            }
          }).join(' ');
          
          if (!shouldSuppress(fullMessage)) {
            originalError(...args);
          }
        };
        
        const wrappedWarn = (...args: any[]) => {
          const fullMessage = args.map(arg => {
            if (typeof arg === 'string') return arg;
            if (arg instanceof Error) return arg.message + ' ' + (arg.stack || '');
            try {
              return String(arg);
            } catch {
              return '';
            }
          }).join(' ');
          
          if (!shouldSuppress(fullMessage)) {
            originalWarn(...args);
          }
        };
        
        // Apply the interception immediately
        console.error = wrappedError;
        console.warn = wrappedWarn;
      }
    },
  };
}
