// passkey(WebAuthn)前端封装。
//
// 只需要这一套实现:iCloud 钥匙串 / Google 密码管理器 / Windows Hello / 小米密码管理 /
// 1Password / 硬件密钥 全都是同一个标准的认证器,浏览器负责弹出系统 UI 让用户选,
// 服务端不感知也不需要对接任何一家。
//
// 不引入 @simplewebauthn/browser:这里只需要 base64url 与 ArrayBuffer 之间的互转,
// 几十行就够,不值得为此多一个依赖。
import { api } from '@/lib/api'

export interface PasskeyItem {
  id: number
  credential_id: string
  name: string
  aaguid: string
  created_at: string
  last_used_at?: string
}

/**
 * passkey 只在 secure context(https,或 localhost)下可用 —— 裸 IP + http 的部署里
 * navigator.credentials 直接不存在。入口必须靠它提前隐藏,否则用户点下去只会拿到
 * 一句看不懂的浏览器报错。
 */
export function passkeySupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    window.isSecureContext &&
    typeof window.PublicKeyCredential !== 'undefined'
  )
}

function base64urlToBuffer(value: string): ArrayBuffer {
  const base64 = value.replace(/-/g, '+').replace(/_/g, '/')
  const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4)
  const binary = atob(padded)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes.buffer
}

function bufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

// 后端(go-webauthn)返回的 options 里二进制字段都是 base64url 字符串,
// 浏览器 API 却要 ArrayBuffer,下面两个函数做这层转换。
type CredentialDescriptorJSON = Omit<PublicKeyCredentialDescriptor, 'id'> & {
  id: string
}

function toDescriptors(
  list?: CredentialDescriptorJSON[]
): PublicKeyCredentialDescriptor[] | undefined {
  return list?.map((item) => ({ ...item, id: base64urlToBuffer(item.id) }))
}

type CreationOptionsJSON = Omit<
  PublicKeyCredentialCreationOptions,
  'challenge' | 'user' | 'excludeCredentials'
> & {
  challenge: string
  user: Omit<PublicKeyCredentialUserEntity, 'id'> & { id: string }
  excludeCredentials?: CredentialDescriptorJSON[]
}

type RequestOptionsJSON = Omit<
  PublicKeyCredentialRequestOptions,
  'challenge' | 'allowCredentials'
> & {
  challenge: string
  allowCredentials?: CredentialDescriptorJSON[]
}

function parseCreationOptions(
  json: CreationOptionsJSON
): PublicKeyCredentialCreationOptions {
  return {
    ...json,
    challenge: base64urlToBuffer(json.challenge),
    user: { ...json.user, id: base64urlToBuffer(json.user.id) },
    excludeCredentials: toDescriptors(json.excludeCredentials),
  }
}

function parseRequestOptions(
  json: RequestOptionsJSON
): PublicKeyCredentialRequestOptions {
  return {
    ...json,
    challenge: base64urlToBuffer(json.challenge),
    allowCredentials: toDescriptors(json.allowCredentials),
  }
}

// 浏览器产出的 credential 反过来要序列化成 base64url JSON 回传给后端。
function serializeCredential(
  credential: PublicKeyCredential,
  response: Record<string, unknown>
) {
  return {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment ?? undefined,
    clientExtensionResults: credential.getClientExtensionResults(),
    response,
  }
}

/**
 * 无密码登录。不需要先填用户名 —— 后端用 discoverable login 发 challenge,
 * 浏览器直接列出本机可用的 passkey 让用户挑,选中即确定了是谁。
 */
export async function loginWithPasskey(rememberMe: boolean) {
  const begin = await api.post('/api/login/passkey/begin', {
    remember_me: rememberMe,
  })
  const { session, options } = begin.data as {
    session: string
    options: RequestOptionsJSON
  }

  const credential = (await navigator.credentials.get({
    publicKey: parseRequestOptions(options),
  })) as PublicKeyCredential | null
  if (!credential) {
    throw new Error('未获取到 passkey')
  }
  const assertion = credential.response as AuthenticatorAssertionResponse
  const payload = serializeCredential(credential, {
    clientDataJSON: bufferToBase64url(assertion.clientDataJSON),
    authenticatorData: bufferToBase64url(assertion.authenticatorData),
    signature: bufferToBase64url(assertion.signature),
    userHandle: assertion.userHandle
      ? bufferToBase64url(assertion.userHandle)
      : undefined,
  })

  // session 走 query,body 保持为浏览器产出的原始 assertion,后端直接按标准格式解析。
  const finish = await api.post(
    `/api/login/passkey/finish?session=${encodeURIComponent(session)}`,
    payload
  )
  return finish.data
}

/** 给当前已登录用户注册一把新 passkey。 */
export async function registerPasskey(name: string) {
  const begin = await api.post('/api/user/passkey/register/begin', {})
  const { session, options } = begin.data as {
    session: string
    options: CreationOptionsJSON
  }

  const credential = (await navigator.credentials.create({
    publicKey: parseCreationOptions(options),
  })) as PublicKeyCredential | null
  if (!credential) {
    throw new Error('未创建 passkey')
  }
  const attestation = credential.response as AuthenticatorAttestationResponse
  const payload = serializeCredential(credential, {
    clientDataJSON: bufferToBase64url(attestation.clientDataJSON),
    attestationObject: bufferToBase64url(attestation.attestationObject),
    transports:
      typeof attestation.getTransports === 'function'
        ? attestation.getTransports()
        : undefined,
  })

  const query = new URLSearchParams({ session, name }).toString()
  await api.post(`/api/user/passkey/register/finish?${query}`, payload)
}

export async function listPasskeys(): Promise<PasskeyItem[]> {
  const response = await api.get('/api/user/passkey')
  return (response.data as { passkeys: PasskeyItem[] }).passkeys ?? []
}

export async function deletePasskey(id: number) {
  await api.delete('/api/user/passkey', { params: { id } })
}
