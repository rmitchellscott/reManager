import type { ReactNode, SVGProps } from 'react'

const deviceOutlines: Record<string, ReactNode> = {
  rm1: (
    <>
      <rect x="64.1" y="32" width="127.8" height="192" rx="16" />
      <line x1="64.1" y1="55.7" x2="191.9" y2="55.7" />
      <line x1="64.1" y1="192" x2="191.9" y2="192" />
    </>
  ),
  rm2: (
    <>
      <rect x="56.9" y="32" width="142.1" height="192" rx="16" />
      <line x1="88.9" y1="32" x2="88.9" y2="224" />
    </>
  ),
  rmpp: (
    <>
      <rect x="61.4" y="32" width="133.2" height="192" rx="16" />
      <line x1="61.4" y1="192" x2="194.6" y2="192" />
    </>
  ),
  rmppmove: (
    <>
      <rect x="78.7" y="32" width="98.6" height="192" rx="16" />
      <line x1="78.7" y1="192" x2="177.3" y2="192" />
    </>
  ),
  rmppure: (
    <>
      <rect x="50.7" y="32" width="154.6" height="192" rx="16" />
      <line x1="82.7" y1="32" x2="82.7" y2="224" />
    </>
  ),
}

interface DeviceIconProps extends SVGProps<SVGSVGElement> {
  device: string
}

export function DeviceIcon({ device, ...svgProps }: DeviceIconProps) {
  const outline = Object.prototype.hasOwnProperty.call(deviceOutlines, device)
    ? deviceOutlines[device]
    : deviceOutlines.rmpp
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 256 256"
      fill="none"
      stroke="currentColor"
      strokeWidth={16}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...svgProps}
    >
      {outline}
    </svg>
  )
}
