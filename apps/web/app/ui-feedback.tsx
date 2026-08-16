"use client";

import { useEffect } from "react";
import { createRandomID } from "./random-id";

function messageFor(input: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement) {
  const explicitLabel = input.id ? document.querySelector<HTMLLabelElement>(`label[for="${CSS.escape(input.id)}"]`)?.textContent?.trim() : "";
  const wrappedLabel = input.closest("label")?.childNodes?.[0]?.textContent?.trim();
  const nearbyLabel = input.closest(".editor-field")?.querySelector<HTMLElement>(".editor-field__label")?.textContent?.trim();
  const label = input.dataset.validationLabel || input.getAttribute("aria-label") || explicitLabel || wrappedLabel || nearbyLabel || "Это поле";
  if (input.validity.valueMissing) return `${label}: заполните это поле.`;
  if (input.validity.typeMismatch) return input.type === "email" ? "Введите корректный email, например name@example.ru." : "Проверьте формат значения.";
  if (input.validity.tooShort && "minLength" in input) return `${label}: минимум ${input.minLength} символов.`;
  if (input.validity.tooLong && "maxLength" in input) return `${label}: максимум ${input.maxLength} символов.`;
  if (input.validity.rangeUnderflow && input instanceof HTMLInputElement) return `${label}: минимальное значение ${input.min}.`;
  if (input.validity.rangeOverflow && input instanceof HTMLInputElement) return `${label}: максимальное значение ${input.max}.`;
  if (input.validity.stepMismatch) return `${label}: выберите допустимое значение.`;
  if (input.validity.patternMismatch) return input.dataset.patternMessage || `${label}: используйте допустимый формат.`;
  return `${label}: проверьте значение.`;
}

function setFieldError(input: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement, message: string) {
  input.setAttribute("aria-invalid", message ? "true" : "false");
  let node = input.parentElement?.querySelector<HTMLElement>(":scope > .field-error");
  if (!message) {
    if (node?.id) {
      const describedBy = (input.getAttribute("aria-describedby") || "").split(/\s+/).filter((id) => id && id !== node?.id);
      if (describedBy.length) input.setAttribute("aria-describedby", describedBy.join(" "));
      else input.removeAttribute("aria-describedby");
    }
    node?.remove();
    return;
  }
  if (!node) {
    node = document.createElement("small");
    node.className = "field-error";
    node.setAttribute("role", "alert");
    input.insertAdjacentElement("afterend", node);
  }
  if (!node.id) node.id = `${input.id || "field"}-${createRandomID().slice(0, 8)}-error`;
  const describedBy = new Set((input.getAttribute("aria-describedby") || "").split(/\s+/).filter(Boolean));
  describedBy.add(node.id);
  input.setAttribute("aria-describedby", Array.from(describedBy).join(" "));
  node.textContent = message;
}

export default function UIFeedback() {
  useEffect(() => {
    let lastFormToastAt = 0;
    let focusScheduled = false;
    const onInvalid = (event: Event) => {
      const input = event.target;
      if (!(input instanceof HTMLInputElement || input instanceof HTMLTextAreaElement || input instanceof HTMLSelectElement)) return;
      event.preventDefault();
      const message = messageFor(input);
      input.setCustomValidity(message);
      setFieldError(input, message);
      if (!focusScheduled) {
        focusScheduled = true;
        requestAnimationFrame(() => {
          focusScheduled = false;
          const first = (input.form || document).querySelector<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>('[aria-invalid="true"]');
          first?.focus({ preventScroll: true });
          first?.scrollIntoView({ block: "center", behavior: "smooth" });
        });
      }
      const now = Date.now();
      if (now - lastFormToastAt > 900) {
        lastFormToastAt = now;
        window.dispatchEvent(new CustomEvent("naimio:toast", { detail: { tone: "error", message: "Проверьте выделенные поля формы" } }));
      }
    };
    const onInput = (event: Event) => {
      const input = event.target;
      if (!(input instanceof HTMLInputElement || input instanceof HTMLTextAreaElement || input instanceof HTMLSelectElement)) return;
      input.setCustomValidity("");
      if (input.validity.valid) setFieldError(input, "");
      else if (input.getAttribute("aria-invalid") === "true") setFieldError(input, messageFor(input));
    };
    document.addEventListener("invalid", onInvalid, true);
    document.addEventListener("input", onInput, true);
    document.addEventListener("change", onInput, true);
    return () => {
      document.removeEventListener("invalid", onInvalid, true);
      document.removeEventListener("input", onInput, true);
      document.removeEventListener("change", onInput, true);
    };
  }, []);
  return null;
}
