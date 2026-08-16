"use client";

import { useEffect } from "react";

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // Surface the error to the browser console for diagnostics; the digest
    // links a client report to the server log without exposing internals.
    console.error(error);
  }, [error]);

  return (
    <main className="status-page">
      <p className="eyebrow">Что-то пошло не так</p>
      <h1>Не удалось загрузить страницу</h1>
      <p className="lead">
        Произошла непредвиденная ошибка. Попробуйте повторить — если проблема
        повторяется, вернитесь немного позже.
      </p>
      <div className="status-page__actions">
        <button onClick={() => reset()}>Повторить попытку</button>
        <a className="button button--quiet" href="/">
          На главную
        </a>
      </div>
      {error.digest ? (
        <p className="card-meta" style={{ marginTop: 18 }}>
          Код обращения: {error.digest}
        </p>
      ) : null}
    </main>
  );
}
