"use client";

import { useToast } from "../../toast";

export default function ShareButton({ title }: { title: string }) {
  const { push } = useToast();

  async function copyLink() {
    const url = window.location.href;
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(url);
      return;
    }
    const textarea = document.createElement("textarea");
    textarea.value = url;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.appendChild(textarea);
    textarea.focus();
    textarea.select();
    const copied = document.execCommand("copy");
    textarea.remove();
    if (!copied) throw new Error("Не удалось скопировать ссылку");
  }

  async function share() {
    const data = { title, text: title, url: window.location.href };
    if (navigator.share) {
      try {
        await navigator.share(data);
        return;
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError") return;
      }
    }
    try {
      await copyLink();
      push({ kind: "success", title: "Ссылка скопирована", message: "Проект можно отправить в любой мессенджер." });
    } catch {
      push({ kind: "error", title: "Не удалось поделиться", message: "Скопируйте адрес страницы из строки браузера." });
    }
  }

  return <button type="button" className="button button--quiet" onClick={share}>Поделиться проектом</button>;
}
