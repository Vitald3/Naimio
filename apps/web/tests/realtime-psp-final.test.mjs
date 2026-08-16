import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const read = (path) => fs.readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');

test('notifications are pushed by websocket without polling loops', () => {
  const badge = read('app/notification-badge.tsx');
  const page = read('app/notifications/page.tsx');
  assert.match(badge, /new WebSocket/);
  assert.match(page, /new WebSocket/);
  assert.doesNotMatch(badge, /setInterval\s*\(/);
  assert.doesNotMatch(page, /setInterval\s*\(/);
  assert.match(badge, /envelope\.data as Preview/);
  assert.match(page, /envelope\.data as Notification/);
});

test('chat receives message payloads over websocket instead of refetching on socket events', () => {
  const chat = read('app/messages/page.tsx');
  assert.match(chat, /envelope\.event === "message\.created"/);
  assert.match(chat, /setMessages\(current => current\.some/);
  const socketSection = chat.slice(chat.indexOf('ws.onmessage'), chat.indexOf('ws.onclose'));
  assert.doesNotMatch(socketSection, /loadMessages\(/);
  assert.doesNotMatch(socketSection, /loadConversations\(/);
});

test('calendar exposes direct month and year selectors', () => {
  const calendar = read('app/pretty-date-input.tsx');
  assert.match(calendar, /aria-label="Месяц"/);
  assert.match(calendar, /aria-label="Год"/);
});

test('completed projects do not render management controls', () => {
  const page = read('app/dashboard/projects/[id]/page.tsx');
  assert.match(page, /\["OPEN","MATCHING","IN_PROGRESS"\]\.includes\(item\.status\)/);
});
