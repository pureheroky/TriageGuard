export type PasswordPolicyRule = {
  id: "min_length" | "uppercase" | "lowercase" | "number" | "symbol";
  label: string;
  ok: boolean;
};

export function getPasswordPolicyRules(password: string): PasswordPolicyRule[] {
  return [
    { id: "min_length", label: "At least 12 characters", ok: password.length >= 12 },
    { id: "uppercase", label: "Contains an uppercase letter", ok: /[A-Z]/.test(password) },
    { id: "lowercase", label: "Contains a lowercase letter", ok: /[a-z]/.test(password) },
    { id: "number", label: "Contains a number", ok: /\d/.test(password) },
    { id: "symbol", label: "Contains a symbol", ok: /[^A-Za-z0-9]/.test(password) },
  ];
}

export function validatePasswordPolicy(password: string): string | null {
  const firstFailed = getPasswordPolicyRules(password).find((rule) => !rule.ok);
  if (!firstFailed) {
    return null;
  }
  switch (firstFailed.id) {
    case "min_length":
      return "Password must be at least 12 characters.";
    case "uppercase":
      return "Password must include an uppercase letter.";
    case "lowercase":
      return "Password must include a lowercase letter.";
    case "number":
      return "Password must include a number.";
    case "symbol":
      return "Password must include a symbol.";
    default:
      return "Password does not meet policy.";
  }
}
