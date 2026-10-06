const http = require('http');

const server = http.createServer((req, res) => {
    // Basic HTTP proxy (not needed for WS but just in case)
    res.writeHead(200);
    res.end('Proxy running');
});

server.on('upgrade', (req, socket, head) => {
    // We are proxying WebSocket connections from port 9092 to 9091
    console.log(`[PROXY] Intercepting WS request to ${req.url}`);

    // Parse URL to check if it's the signaling publisher
    const url = new URL(`http://localhost${req.url}`);
    
    // Create the proxy request headers
    const proxyHeaders = [
        `GET ${req.url} HTTP/1.1`,
        `Host: localhost:9091`,
        `Upgrade: websocket`,
        `Connection: Upgrade`,
        `Sec-WebSocket-Key: ${req.headers['sec-websocket-key']}`,
        `Sec-WebSocket-Version: 13`
    ];

    // If it's the publisher, inject the Authorization header!
    if (req.url.includes('/signaling')) {
        // The token is in the query params. We extract it and put it in the header
        const token = url.searchParams.get('token');
        if (token) {
            proxyHeaders.push(`Authorization: Bearer ${token}`);
            console.log(`[PROXY] Injected Authorization header with token: ${token}`);
        }
    }

    // Add any other original headers
    if (req.headers['sec-websocket-protocol']) {
        proxyHeaders.push(`Sec-WebSocket-Protocol: ${req.headers['sec-websocket-protocol']}`);
    }

    const proxyReq = proxyHeaders.join('\r\n') + '\r\n\r\n';

    // Connect to the REAL backend on port 9091
    const net = require('net');
    const proxySocket = net.connect(9091, '127.0.0.1', () => {
        console.log(`[PROXY] Connected to backend 9091`);
        // Send the modified handshake
        proxySocket.write(proxyReq);
        // Forward any body/head
        if (head && head.length > 0) proxySocket.write(head);
        
        // Pipe the sockets together bidirectionally
        socket.pipe(proxySocket);
        proxySocket.pipe(socket);
    });

    proxySocket.on('error', (err) => {
        console.error('[PROXY] Backend connection error:', err.message);
        socket.end();
    });

    socket.on('error', (err) => {
        console.error('[PROXY] Client socket error:', err.message);
        proxySocket.end();
    });
});

server.listen(9092, () => {
    console.log('==============================================');
    console.log('   Pulse Header Injection Proxy (Node.js)     ');
    console.log('   Listening on ws://localhost:9092           ');
    console.log('   Forwarding to ws://localhost:9091          ');
    console.log('==============================================');
});
