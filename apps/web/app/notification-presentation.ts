export type NotificationPresentation = { title: string; hint: string };

const presentations: Record<string, NotificationPresentation> = {
  new_project_available:{title:"Подборка новых проектов",hint:"Откройте каталог, чтобы посмотреть новые задачи и бюджеты."},
  new_vacancy_available:{title:"Подборка новых вакансий",hint:"Откройте каталог, чтобы посмотреть новые требования и условия."},
  new_service_available:{title:"Подборка новых услуг",hint:"Откройте каталог новых услуг, консультаций и обучения."},
  new_message:{title:"Новое сообщение",hint:"Вам пришло сообщение в диалоге."},
  "message.created":{title:"Новое сообщение",hint:"Вам пришло сообщение в диалоге."},
  proposal_received:{title:"Новый отклик на проект",hint:"Исполнитель откликнулся на ваш проект."},
  "proposal.created":{title:"Новый отклик на проект",hint:"Исполнитель откликнулся на ваш проект."},
  project_status_changed:{title:"Статус проекта изменился",hint:"Обновился статус одного из ваших проектов."},
  new_review_received:{title:"Новый отзыв",hint:"Вам оставили отзыв о сотрудничестве."},
  "review.created":{title:"Новый отзыв",hint:"Вам оставили отзыв о сотрудничестве."},
  deal_funded:{title:"Безопасная сделка оплачена",hint:"Средства зарезервированы, можно начинать работу."},
  "deal.funded":{title:"Безопасная сделка оплачена",hint:"Средства зарезервированы, можно начинать работу."},
  external_reputation_verified:{title:"Внешняя репутация подтверждена",hint:"Импорт репутации с другой площадки подтверждён."},
  invite_accepted:{title:"Приглашение принято",hint:"Ваше приглашение было принято."},
  invited_to_project:{title:"Приглашение в проект",hint:"Вас пригласили поработать над проектом."},
  reward_granted:{title:"Начислена промо-награда",hint:"Начислена промо-льгота по программе развития."},
  moderation_update:{title:"Решение модерации",hint:"Поддержка Naimio сообщила о решении по вашему материалу."},
};

export function notificationPresentation(type: string): NotificationPresentation {
  return presentations[type] ?? { title: "Обновление на платформе", hint: "Подробности доступны в связанном разделе кабинета." };
}
