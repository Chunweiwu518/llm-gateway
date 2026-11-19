"""Print the current TOTP code for a base32 secret (demo helper, stdlib only)."""
import base64, hashlib, hmac, struct, sys, time

secret = sys.argv[1].strip().upper()
key = base64.b32decode(secret + "=" * (-len(secret) % 8))
msg = struct.pack(">Q", int(time.time()) // 30)
h = hmac.new(key, msg, hashlib.sha1).digest()
o = h[-1] & 0x0F
print(f"{(struct.unpack('>I', h[o:o + 4])[0] & 0x7FFFFFFF) % 1_000_000:06d}")
