type TriageGuardLogoProps = {
  size?: number;
  className?: string;
};

export function TriageGuardLogo({ size = 48, className = "" }: TriageGuardLogoProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
    >
      <path
        d="M32 6L10 17V31C10 43.8 19.6 55.6 32 59C44.4 55.6 54 43.8 54 31V17L32 6Z"
        fill="none"
        stroke="currentColor"
        strokeWidth="3"
        strokeLinejoin="round"
      />
      <g stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
        <circle cx="32" cy="30" r="2.5" fill="currentColor" stroke="none" />
        <line x1="32" y1="27.5" x2="32" y2="19" />
        <line x1="29.8" y1="31.8" x2="23" y2="38.5" />
        <line x1="34.2" y1="31.8" x2="41" y2="38.5" />
      </g>
      <polyline
        points="28,44 31,47 37,41"
        stroke="currentColor"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        fill="none"
      />
    </svg>
  );
}
