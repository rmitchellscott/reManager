declare module '*/wailsjs/go/main/App' {
  export function Connect(host: string, password: string): Promise<{
    success: boolean
    message: string
    device?: string
  }>
  export function ConnectWithAuth(
    host: string,
    authType: string,
    secret: string,
    keyPath: string
  ): Promise<{
    success: boolean
    message: string
    device?: string
  }>
  export function ConnectWithKey(
    host: string,
    keyPath: string,
    passphrase: string
  ): Promise<{
    success: boolean
    message: string
    device?: string
  }>
  export function ConnectWithAgent(host: string): Promise<{
    success: boolean
    message: string
    device?: string
  }>
  export function IsSSHAgentAvailable(): Promise<boolean>
  export function CancelConnect(): Promise<void>
  export function Disconnect(): Promise<void>
  export function IsConnected(): Promise<boolean>
  export function RunCommand(cmd: string): Promise<string>
  export function RunCommandWithOutput(cmd: string): Promise<void>
  export function StopCommand(): Promise<void>
  export function GetDeviceInfo(): Promise<Record<string, string>>
  export function GetUpdateServiceStatus(): Promise<{
    enabled: boolean
    running: boolean
  }>
  export function GetDefaultSSHKeys(): Promise<
    Array<{ path: string; name: string }>
  >
  export function SelectKeyFile(): Promise<string>
  export function CheckComponentStatus(componentId: string): Promise<boolean>
}

declare module '*/wailsjs/runtime/runtime' {
  export function EventsOn(eventName: string, callback: (...args: any[]) => void): () => void
  export function EventsOff(eventName: string): void
  export function EventsEmit(eventName: string, ...args: any[]): void
}
