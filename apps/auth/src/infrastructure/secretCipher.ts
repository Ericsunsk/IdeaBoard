import { createCipheriv, createDecipheriv, randomBytes } from "node:crypto";

export type SecretCipher = {
  encrypt(plaintext: string): string;
  decrypt(encoded: string): string;
};

export function createSecretCipher(encodedKey: string): SecretCipher {
  const key = encodedKey.trim();
  return {
    encrypt: (plaintext) => encryptSecret(plaintext, key),
    decrypt: (encoded) => decryptSecret(encoded, key),
  };
}

export function encryptSecret(plaintext: string, encodedKey: string): string {
  const iv = randomBytes(12);
  const cipher = createCipheriv("aes-256-gcm", encryptionKeyBytes(encodedKey), iv);
  const ciphertext = Buffer.concat([
    cipher.update(plaintext, "utf8"),
    cipher.final(),
  ]);
  return Buffer.concat([iv, cipher.getAuthTag(), ciphertext]).toString("base64");
}

export function decryptSecret(encoded: string, encodedKey: string): string {
  const payload = Buffer.from(encoded, "base64");
  if (payload.length < 28) throw new Error("invalid ciphertext");
  const decipher = createDecipheriv(
    "aes-256-gcm",
    encryptionKeyBytes(encodedKey),
    payload.subarray(0, 12),
  );
  decipher.setAuthTag(payload.subarray(12, 28));
  return Buffer.concat([
    decipher.update(payload.subarray(28)),
    decipher.final(),
  ]).toString("utf8");
}

export function isValidSecretEncryptionKey(value: string): boolean {
  try {
    encryptionKeyBytes(value);
    return true;
  } catch {
    return false;
  }
}

function encryptionKeyBytes(value: string): Buffer {
  const raw = Buffer.from(value.trim(), "base64");
  if (raw.length !== 32) {
    throw new Error("secret encryption key must decode to exactly 32 bytes");
  }
  return raw;
}
