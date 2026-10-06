const http = require('http');
const WebSocket = require('ws');
const fs = require('fs');
const path = require('path');

const server = http.createServer((req, res) => {
    console.log(`[HTTP] GET ${req.url} from ${req.socket.remoteAddress}`);
    let filePath = path.join(__dirname, 'web_remote', req.url === '/' ? 'index.html' : req.url);
    const extname = path.extname(filePath);
    let contentType = 'text/html';
    switch (extname) {
        case '.js': contentType = 'text/javascript'; break;
        case '.css': contentType = 'text/css'; break;
        case '.json': contentType = 'application/json'; break;
        case '.png': contentType = 'image/png'; break;
        case '.jpg': contentType = 'image/jpg'; break;
    }

    fs.readFile(filePath, (error, content) => {
        if (error) {
            if (error.code == 'ENOENT') {
                res.writeHead(404);
                res.end('404 Not Found');
            } else {
                res.writeHead(500);
                res.end('500 Server Error: ' + error.code);
            }
        } else {
            res.writeHead(200, { 'Content-Type': contentType });
            res.end(content, 'utf-8');
        }
    });
});

const wss = new WebSocket.Server({ server });

let viewerWs = null;
let publisherWs = null;
let publisherOfferBuffer = [];

wss.on('connection', (ws, req) => {
    console.log(`[+] New Connection: ${req.url}`);

    if (req.url.includes('/client')) {
        viewerWs = ws;
        console.log('    -> Viewer Registered');
        // Drain buffer
        while (publisherOfferBuffer.length > 0) {
            const msg = publisherOfferBuffer.shift();
            viewerWs.send(msg);
            console.log(`[Buffer] Flushed to Viewer`);
        }
    } else if (req.url.includes('/signaling')) {
        publisherWs = ws;
        publisherOfferBuffer = []; // Reset buffer on new publisher
        console.log('    -> Publisher Registered');
    }

    ws.on('message', (message) => {
        const msgStr = message.toString();
        console.log(`[Data] from ${req.url.includes('/client') ? 'Viewer' : 'Publisher'}: ${msgStr.substring(0, 60)}...`);

        if (req.url.includes('/client')) {
            if (publisherWs && publisherWs.readyState === WebSocket.OPEN) {
                publisherWs.send(msgStr);
            }
        } else if (req.url.includes('/signaling')) {
            if (viewerWs && viewerWs.readyState === WebSocket.OPEN) {
                viewerWs.send(msgStr);
            } else {
                console.log(`[Buffer] Queued from Publisher`);
                publisherOfferBuffer.push(msgStr);
            }
        }
    });

    ws.on('close', () => {
        console.log(`[-] Disconnected: ${req.url}`);
        if (req.url.includes('/client')) viewerWs = null;
        if (req.url.includes('/signaling')) {
            publisherWs = null;
            publisherOfferBuffer = [];
        }
    });
});

server.listen(8000, () => {
    console.log('==========================================');
    console.log('   Pulse Mock Cloud Server (Node.js)');
    console.log('==========================================');
    console.log('Listening on http://localhost:8000...');
});
