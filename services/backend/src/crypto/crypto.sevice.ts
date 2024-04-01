import { Injectable } from "@nestjs/common"
import { createCipheriv, createDecipheriv, getHashes, randomBytes, scryptSync } from "crypto"

@Injectable()
export class CryptoService {
    private algoritm = "aes-256-cbc"
    private key: Buffer
    private iv: Buffer

    constructor() {
        this.iv = randomBytes(16)
        this.key = scryptSync("anime", "enigma", 32)
    }

    encrypt(text: string): string {
        const cipher = createCipheriv(this.algoritm, this.key, this.iv)
        let encrypted = cipher.update(text, "utf8", "hex")
        encrypted += cipher.final("hex")
        return encrypted
    }

    decrypt(text: string): string {
        const cipher = createDecipheriv(this.algoritm, this.key, this.iv)
        let decrypted = cipher.update(text, "hex", "utf8")
        decrypted += cipher.final("utf8")
        return decrypted
    }
}