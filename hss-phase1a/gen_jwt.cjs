const crypto = require('crypto');

// 1. Generate Ed25519 Keypair
const { publicKey, privateKey } = crypto.generateKeyPairSync('ed25519');
const pubKeyPem = publicKey.export({ type: 'spki', format: 'pem' });
console.log("--- PUBLIC KEY ---");
console.log(pubKeyPem);

// 2. Create JWT Header and Payload
const header = { alg: "EdDSA", typ: "JWT" };
const payload = {
    sub: "test-camera-1",
    role: "device",
    iat: Math.floor(Date.now() / 1000),
    exp: Math.floor(Date.now() / 1000) + (365 * 24 * 60 * 60)
};

const base64UrlEncode = (obj) => Buffer.from(JSON.stringify(obj)).toString('base64').replace(/=/g, '').replace(/\+/g, '-').replace(/\//g, '_');

const encodedHeader = base64UrlEncode(header);
const encodedPayload = base64UrlEncode(payload);
const signingInput = `${encodedHeader}.${encodedPayload}`;

// 3. Sign the JWT
const signature = crypto.sign(null, Buffer.from(signingInput), privateKey);
const encodedSignature = signature.toString('base64').replace(/=/g, '').replace(/\+/g, '-').replace(/\//g, '_');

const jwt = `${signingInput}.${encodedSignature}`;

console.log("--- JWT TOKEN ---");
console.log(jwt);

// 4. Print the SQL Command
const sql = `INSERT INTO devices (id, public_key, created_at) VALUES ('test-camera-1', '${pubKeyPem.replace(/\n/g, '\\n')}', NOW()) ON CONFLICT (id) DO UPDATE SET public_key = EXCLUDED.public_key;`;
console.log("--- SQL COMMAND ---");
console.log(sql);
