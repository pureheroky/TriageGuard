import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";

function expectForwardRefComponents(relativePath, componentNames) {
  const filePath = path.resolve(process.cwd(), relativePath);
  const source = fs.readFileSync(filePath, "utf8");

  for (const componentName of componentNames) {
    assert.match(
      source,
      new RegExp(`const\\s+${componentName}\\s*=\\s*React\\.forwardRef<`, "m"),
      `${relativePath}: ${componentName} must use React.forwardRef`,
    );
    assert.match(
      source,
      new RegExp(`${componentName}\\.displayName\\s*=`, "m"),
      `${relativePath}: ${componentName} should set a displayName`,
    );
    assert.doesNotMatch(
      source,
      new RegExp(`function\\s+${componentName}\\s*\\(`, "m"),
      `${relativePath}: ${componentName} must not remain a plain function component`,
    );
  }
}

test("Sheet wrappers that receive refs use React.forwardRef", () => {
  expectForwardRefComponents("components/ui/sheet.tsx", ["SheetOverlay", "SheetContent", "SheetTitle", "SheetDescription"]);
});

test("Other shared radix wrappers use React.forwardRef where refs are expected", () => {
  expectForwardRefComponents("components/ui/avatar.tsx", ["Avatar", "AvatarImage", "AvatarFallback"]);
  expectForwardRefComponents("components/ui/label.tsx", ["Label"]);
  expectForwardRefComponents("components/ui/switch.tsx", ["Switch"]);
  expectForwardRefComponents("components/ui/accordion.tsx", ["AccordionItem", "AccordionTrigger", "AccordionContent"]);
});
