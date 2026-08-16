DO $$
DECLARE
  emails text[]:=ARRAY['admin@example.test','moderator@example.test','customer@example.test','customer2@example.test','customer3@example.test','customer4@example.test','customer5@example.test','customer6@example.test','freelancer@example.test','flutter@example.test','go@example.test','fullstack@example.test','designer@example.test','ml@example.test','seo@example.test','marketing@example.test','copywriter@example.test','marketplace@example.test'];
  names text[]:=ARRAY['Анна Администратор','Михаил Модератор','Елена Соколова','Алексей Воронцов','Мария Белова','Илья Миронов','Дарья Орлова','Сергей Климов','Максим Кузнецов','Алина Волкова','Иван Петров','Никита Смирнов','Софья Морозова','Роман Лебедев','Екатерина Новикова','Павел Фёдоров','Ольга Васильева','Артём Попов'];
  usernames text[]:=ARRAY['admin-demo','moderator-demo','customer-demo','customer-two','customer-three','customer-four','customer-five','customer-six','telegram-dev','flutter-dev','go-backend','fullstack-pro','ux-designer','ml-engineer','seo-expert','performance-pro','copywriter-pro','marketplace-manager'];
  i int;
BEGIN
  FOR i IN 1..array_length(emails,1) LOOP
    INSERT INTO users(id,email,email_normalized,password_hash,username,username_normalized,display_name,email_verified_at)
    VALUES(md5('seed:user:'||emails[i])::uuid,emails[i],emails[i],current_setting('freelance.seed_hash'),usernames[i],usernames[i],names[i],now())
    ON CONFLICT(email_normalized) WHERE deleted_at IS NULL DO UPDATE SET password_hash=EXCLUDED.password_hash,display_name=EXCLUDED.display_name,username=EXCLUDED.username,username_normalized=EXCLUDED.username_normalized,status='ACTIVE';
  END LOOP;
END $$;
INSERT INTO user_roles(user_id,role,granted_by) VALUES
 (md5('seed:user:admin@example.test')::uuid,'ADMIN',md5('seed:user:admin@example.test')::uuid),
 (md5('seed:user:moderator@example.test')::uuid,'MODERATOR',md5('seed:user:admin@example.test')::uuid)
ON CONFLICT DO NOTHING;
-- Staff identities are deliberately separate from marketplace capabilities.
DELETE FROM user_capabilities WHERE user_id IN (
 md5('seed:user:admin@example.test')::uuid,
 md5('seed:user:moderator@example.test')::uuid
);
INSERT INTO user_capabilities(user_id,capability)
SELECT md5('seed:user:'||email)::uuid,capability FROM
 (VALUES('customer@example.test'),('customer2@example.test'),('customer3@example.test'),('customer4@example.test'),('customer5@example.test'),('customer6@example.test')) u(email)
 CROSS JOIN (VALUES('CUSTOMER')) c(capability) ON CONFLICT DO NOTHING;
INSERT INTO user_capabilities(user_id,capability)
SELECT md5('seed:user:'||email)::uuid,'FREELANCER' FROM (VALUES('freelancer@example.test'),('flutter@example.test'),('go@example.test'),('fullstack@example.test'),('designer@example.test'),('ml@example.test'),('seo@example.test'),('marketing@example.test'),('copywriter@example.test'),('marketplace@example.test')) u(email) ON CONFLICT DO NOTHING;

INSERT INTO categories(id,slug,name,description,sort_order) VALUES
 (md5('seed:cat:development')::uuid,'development','IT и разработка','Веб, мобильные приложения, backend и интеграции',10),
 (md5('seed:cat:design')::uuid,'design','Дизайн','Интерфейсы, брендинг и визуальные коммуникации',20),
 (md5('seed:cat:ai')::uuid,'ai','AI и машинное обучение','ML, LLM и автоматизация процессов',30),
 (md5('seed:cat:marketing')::uuid,'marketing','Маркетинг','Продвижение, аналитика и реклама',40),
 (md5('seed:cat:seo')::uuid,'seo','SEO','Поисковая оптимизация и контент-стратегия',50),
 (md5('seed:cat:marketplaces')::uuid,'marketplaces','Маркетплейсы','Управление продажами на маркетплейсах',60),
 (md5('seed:cat:content')::uuid,'content','Контент','Тексты, редактура и коммуникации',70)
ON CONFLICT(slug) DO UPDATE SET name=EXCLUDED.name,description=EXCLUDED.description,is_active=true;
INSERT INTO skills(id,slug,name) VALUES
 (md5('seed:skill:go')::uuid,'go','Go'),(md5('seed:skill:postgresql')::uuid,'postgresql','PostgreSQL'),(md5('seed:skill:flutter')::uuid,'flutter','Flutter'),
 (md5('seed:skill:typescript')::uuid,'typescript','TypeScript'),(md5('seed:skill:react')::uuid,'react','React'),(md5('seed:skill:nextjs')::uuid,'nextjs','Next.js'),
 (md5('seed:skill:figma')::uuid,'figma','Figma'),(md5('seed:skill:ux')::uuid,'ux-research','UX Research'),(md5('seed:skill:python')::uuid,'python','Python'),
 (md5('seed:skill:ml')::uuid,'machine-learning','Machine Learning'),(md5('seed:skill:seo')::uuid,'seo-audit','SEO-аудит'),(md5('seed:skill:analytics')::uuid,'web-analytics','Веб-аналитика'),
 (md5('seed:skill:ads')::uuid,'performance-ads','Performance-реклама'),(md5('seed:skill:copy')::uuid,'copywriting','Копирайтинг'),(md5('seed:skill:wb')::uuid,'wildberries','Wildberries'),
 (md5('seed:skill:telegram')::uuid,'telegram-bots','Telegram-боты')
ON CONFLICT(slug) DO UPDATE SET name=EXCLUDED.name,is_active=true;
INSERT INTO category_skills(category_id,skill_id)
SELECT md5('seed:cat:'||c)::uuid,md5('seed:skill:'||s)::uuid FROM (VALUES
 ('development','go'),('development','postgresql'),('development','flutter'),('development','typescript'),('development','react'),('development','nextjs'),('development','telegram'),
 ('design','figma'),('design','ux'),('ai','python'),('ai','ml'),('seo','seo'),('seo','analytics'),('marketing','ads'),('marketing','analytics'),('content','copy'),('marketplaces','wb')) x(c,s)
ON CONFLICT DO NOTHING;

DO $$ DECLARE emails text[]:=ARRAY['freelancer@example.test','flutter@example.test','go@example.test','fullstack@example.test','designer@example.test','ml@example.test','seo@example.test','marketing@example.test','copywriter@example.test','marketplace@example.test']; titles text[]:=ARRAY['Telegram bot developer','Flutter разработчик','Go backend разработчик','Full-stack разработчик','UI/UX дизайнер','ML/AI разработчик','SEO специалист','Performance-маркетолог','Копирайтер и редактор','Marketplace manager']; cats text[]:=ARRAY['development','development','development','development','design','ai','seo','marketing','content','marketplaces']; i int; BEGIN
 FOR i IN 1..10 LOOP INSERT INTO professional_profiles(user_id,professional_title,bio,location_text,country_code,experience_years,hourly_rate_kopecks,minimum_order_kopecks,availability,profile_visibility,response_time_minutes,profile_completion)
 VALUES(md5('seed:user:'||emails[i])::uuid,titles[i],titles[i]||'. Работаю с продуктовым бизнесом, прозрачно оцениваю сроки и показываю результат по этапам.','Москва, Россия','RU',2+i,120000+i*25000,500000+i*100000,CASE WHEN i%3=0 THEN 'PARTIALLY_BUSY' ELSE 'AVAILABLE' END,'PUBLIC',15+i*7,80+i%3*10)
 ON CONFLICT(user_id) DO UPDATE SET professional_title=EXCLUDED.professional_title,bio=EXCLUDED.bio,availability=EXCLUDED.availability,profile_visibility='PUBLIC',profile_completion=EXCLUDED.profile_completion;
 INSERT INTO profile_categories(user_id,category_id,is_primary) VALUES(md5('seed:user:'||emails[i])::uuid,md5('seed:cat:'||cats[i])::uuid,true) ON CONFLICT DO NOTHING;
 END LOOP; END $$;
INSERT INTO profile_skills(user_id,skill_id,level,years,is_featured)
SELECT md5('seed:user:'||email)::uuid,md5('seed:skill:'||skill)::uuid,'EXPERT',5,true FROM (VALUES
 ('freelancer@example.test','telegram'),('flutter@example.test','flutter'),('go@example.test','go'),('go@example.test','postgresql'),('fullstack@example.test','typescript'),('fullstack@example.test','react'),('designer@example.test','figma'),('designer@example.test','ux'),('ml@example.test','python'),('ml@example.test','ml'),('seo@example.test','seo'),('marketing@example.test','ads'),('copywriter@example.test','copy'),('marketplace@example.test','wb')) x(email,skill) ON CONFLICT DO NOTHING;
INSERT INTO profile_languages(user_id,language_code,level) SELECT id,'ru','NATIVE' FROM users WHERE email_normalized LIKE '%@example.test' AND id IN(SELECT user_id FROM professional_profiles) ON CONFLICT DO NOTHING;

DO $$ DECLARE emails text[]:=ARRAY['freelancer@example.test','flutter@example.test','go@example.test','fullstack@example.test','designer@example.test','ml@example.test','seo@example.test','marketing@example.test']; i int;j int; BEGIN FOR i IN 1..8 LOOP FOR j IN 1..3 LOOP
 INSERT INTO portfolio_items(id,user_id,title,slug,description,price_min_kopecks,price_max_kopecks,completed_on,visibility,sort_order)
 VALUES(md5('seed:portfolio:'||i||':'||j)::uuid,md5('seed:user:'||emails[i])::uuid,'Кейс '||j||': результат для продуктовой команды','case-'||i||'-'||j,'Задача, принятые решения, процесс и измеримый результат проекта без раскрытия данных клиента.',500000*j,900000*j,current_date-(i*j*17),'PUBLIC',j)
 ON CONFLICT(id) DO UPDATE SET title=EXCLUDED.title,description=EXCLUDED.description,visibility='PUBLIC'; END LOOP; END LOOP; END $$;

DO $$ DECLARE sellers text[]:=ARRAY['freelancer@example.test','flutter@example.test','go@example.test','fullstack@example.test','designer@example.test','ml@example.test','seo@example.test','marketing@example.test','copywriter@example.test','marketplace@example.test']; titles text[]:=ARRAY['Telegram-бот для заявок и поддержки','Аудит Flutter-приложения','Архитектура Go API','Разработка продуктового MVP','UX-аудит сервиса','Прототип ML-модели','Технический SEO-аудит','Настройка performance-рекламы','Редактура лендинга','Аудит карточек Wildberries','Консультация по backend','Менторинг по Flutter','Обучение Figma для команды','Практикум по веб-аналитике','SEO-стратегия на квартал','Сопровождение запуска MVP']; cats text[]:=ARRAY['development','development','development','development','design','ai','seo','marketing','content','marketplaces','development','development','design','marketing','seo','development']; i int; typ text; BEGIN FOR i IN 1..16 LOOP typ:=CASE WHEN i=12 THEN 'MENTORING' WHEN i IN(13,14) THEN 'EDUCATION' WHEN i IN(2,3,5,11,15) THEN 'CONSULTATION' ELSE 'PROFESSIONAL_SERVICE' END;
 INSERT INTO services(id,seller_user_id,category_id,service_type,title,slug,short_description,description,price_type,price_from_kopecks,delivery_days,included_revisions,status,visibility,published_at)
 VALUES(md5('seed:service:'||i)::uuid,md5('seed:user:'||sellers[1+(i-1)%10])::uuid,md5('seed:cat:'||cats[i])::uuid,typ,titles[i],'demo-service-'||i,'Понятный объём работ, сроки и ожидаемый результат.','Согласуем задачу, фиксируем состав результата и работаем по этапам. В стоимость входит итоговая передача материалов и рекомендации.','FROM',700000+i*180000,3+i%14,2,'ACTIVE','PUBLIC',now()-i*interval '1 day') ON CONFLICT(id) DO UPDATE SET title=EXCLUDED.title,status='ACTIVE',published_at=EXCLUDED.published_at;
 IF typ IN('EDUCATION','MENTORING') THEN INSERT INTO education_service_details(service_id,format,duration_minutes,sessions_count,audience_type,group_size_max) VALUES(md5('seed:service:'||i)::uuid,'ONLINE',90,CASE WHEN typ='MENTORING' THEN 6 ELSE 3 END,'INDIVIDUAL',NULL) ON CONFLICT(service_id) DO UPDATE SET duration_minutes=EXCLUDED.duration_minutes; END IF; END LOOP; END $$;

DO $$ DECLARE customers text[]:=ARRAY['customer@example.test','customer2@example.test','customer3@example.test','customer4@example.test','customer5@example.test','customer6@example.test']; titles text[]:=ARRAY['Разработать Telegram-бот для отдела продаж','Мобильное приложение для сервиса доставки','Go API для личного кабинета','Редизайн интерфейса B2B-платформы','Настроить ML-классификацию обращений','SEO-аудит интернет-магазина','Запустить performance-кампанию','Подготовить тексты для нового продукта','Оптимизировать карточки на Wildberries','Собрать MVP на Next.js','Провести UX-исследование','Интегрировать PostgreSQL и отчёты','Доработать Flutter-приложение','Разработать дизайн-систему','Настроить аналитику воронки','Автоматизировать обработку лидов','Создать корпоративный сайт','Провести консультацию по архитектуре','Подготовить SEO-стратегию','Запустить кабинет партнёра']; cats text[]:=ARRAY['development','development','development','design','ai','seo','marketing','content','marketplaces','development','design','development','development','design','marketing','development','development','development','seo','development']; i int; st text; BEGIN FOR i IN 1..20 LOOP st:=CASE WHEN i<=14 THEN 'OPEN' WHEN i=15 THEN 'MATCHING' WHEN i IN(16,17,18,19) THEN 'IN_PROGRESS' ELSE 'COMPLETED' END;
 INSERT INTO projects(id,customer_user_id,category_id,title,slug,description,budget_type,budget_min_kopecks,budget_max_kopecks,deadline_at,experience_level,visibility,status,source_type,published_at,proposal_count)
 VALUES(md5('seed:project:'||i)::uuid,md5('seed:user:'||customers[1+(i-1)%6])::uuid,md5('seed:cat:'||cats[i])::uuid,titles[i],'demo-project-'||i,'Нужен специалист, который поможет уточнить требования, предложит план и доведёт задачу до проверяемого результата. В отклике важны релевантные примеры и оценка сроков.','RANGE',1500000+i*120000,2500000+i*180000,now()+(14+i)*interval '1 day','INTERMEDIATE','PUBLIC',st,'MANUAL',now()-i*interval '5 hour',CASE WHEN i<=14 THEN 3 ELSE 1 END) ON CONFLICT(id) DO UPDATE SET title=EXCLUDED.title,status=EXCLUDED.status,published_at=EXCLUDED.published_at,visibility='PUBLIC';
 END LOOP; END $$;
INSERT INTO project_skills(project_id,skill_id,importance) SELECT md5('seed:project:'||i)::uuid,md5('seed:skill:'||skill)::uuid,100 FROM (VALUES(1,'telegram'),(2,'flutter'),(3,'go'),(4,'figma'),(5,'ml'),(6,'seo'),(7,'ads'),(8,'copy'),(9,'wb'),(10,'nextjs'),(11,'ux'),(12,'postgresql'),(13,'flutter'),(14,'figma'),(15,'analytics'),(16,'telegram'),(17,'react'),(18,'go'),(19,'seo'),(20,'typescript')) x(i,skill) ON CONFLICT DO NOTHING;

DO $$ DECLARE i int;j int; freelancers text[]:=ARRAY['freelancer@example.test','flutter@example.test','go@example.test','fullstack@example.test','designer@example.test','ml@example.test','seo@example.test','marketing@example.test','copywriter@example.test','marketplace@example.test']; BEGIN FOR i IN 1..14 LOOP FOR j IN 1..3 LOOP INSERT INTO proposals(id,project_id,freelancer_user_id,message,price_kopecks,delivery_days,status) VALUES(md5('seed:proposal:'||i||':'||j)::uuid,md5('seed:project:'||i)::uuid,md5('seed:user:'||freelancers[1+(i+j-2)%10])::uuid,'Изучил задачу. Предлагаю начать с короткого уточнения требований, затем показать промежуточный результат и завершить работу в согласованный срок.',1600000+i*100000+j*150000,5+j*2,CASE WHEN j=2 AND i%3=0 THEN 'SHORTLISTED' ELSE 'PENDING' END) ON CONFLICT(project_id,freelancer_user_id) DO UPDATE SET message=EXCLUDED.message,price_kopecks=EXCLUDED.price_kopecks; END LOOP; END LOOP; FOR i IN 15..20 LOOP INSERT INTO proposals(id,project_id,freelancer_user_id,message,price_kopecks,delivery_days,status) VALUES(md5('seed:proposal:'||i||':1')::uuid,md5('seed:project:'||i)::uuid,md5('seed:user:'||freelancers[1+(i-15)%10])::uuid,'Согласованный демонстрационный отклик для проверки полного сценария.',2000000+i*100000,10,'ACCEPTED') ON CONFLICT(project_id,freelancer_user_id) DO NOTHING; INSERT INTO project_assignments(id,project_id,proposal_id,freelancer_user_id,agreed_price_kopecks,currency,status,started_at,completed_at) VALUES(md5('seed:assignment:'||i)::uuid,md5('seed:project:'||i)::uuid,md5('seed:proposal:'||i||':1')::uuid,md5('seed:user:'||freelancers[1+(i-15)%10])::uuid,2000000+i*100000,'RUB',CASE WHEN i=20 THEN 'COMPLETED' ELSE 'ACTIVE' END,CASE WHEN i=15 THEN NULL ELSE now()-interval '5 days' END,CASE WHEN i=20 THEN now()-interval '1 day' END) ON CONFLICT(proposal_id) WHERE proposal_id IS NOT NULL DO NOTHING; END LOOP; END $$;
UPDATE safe_deals SET status=CASE substring(project_snapshot->>'title' from '.*') WHEN '' THEN status ELSE status END;
UPDATE safe_deals d SET status=x.status,funded_at=CASE WHEN x.status<>'AWAITING_FUNDING' THEN now()-interval '6 days' END,work_started_at=CASE WHEN x.status IN('IN_PROGRESS','SUBMITTED','REVISION_REQUESTED','DISPUTED','COMPLETED') THEN now()-interval '5 days' END,submitted_at=CASE WHEN x.status IN('SUBMITTED','REVISION_REQUESTED','DISPUTED','COMPLETED') THEN now()-interval '2 days' END,completed_at=CASE WHEN x.status='COMPLETED' THEN now()-interval '1 day' END FROM (VALUES(15,'AWAITING_FUNDING'),(16,'REVISION_REQUESTED'),(17,'IN_PROGRESS'),(18,'SUBMITTED'),(19,'DISPUTED'),(20,'COMPLETED')) x(n,status) WHERE d.project_id=md5('seed:project:'||x.n)::uuid;
INSERT INTO payment_records(id,deal_id,provider,provider_payment_id,provider_status,amount_kopecks,currency,idempotency_key)
SELECT md5('seed:payment:'||x.n)::uuid,d.id,'sandbox','seed-payment-'||x.n,CASE WHEN x.n=15 THEN 'PENDING' WHEN x.n=20 THEN 'RELEASED' ELSE 'FUNDED' END,d.gross_amount_kopecks,'RUB','seed-payment-'||x.n FROM safe_deals d JOIN (VALUES(15),(16),(17),(18),(19),(20)) x(n) ON d.project_id=md5('seed:project:'||x.n)::uuid ON CONFLICT(deal_id,idempotency_key) DO NOTHING;
INSERT INTO safe_deal_submissions(id,deal_id,revision_number,submitted_by_user_id,summary) SELECT md5('seed:submission:'||x.n)::uuid,d.id,0,d.freelancer_user_id,'Работа передана: материалы, инструкция и итоговые файлы приложены в переписке.' FROM safe_deals d JOIN (VALUES(16),(18),(19),(20))x(n) ON d.project_id=md5('seed:project:'||x.n)::uuid ON CONFLICT(deal_id,revision_number) DO NOTHING;
INSERT INTO safe_deal_disputes(id,deal_id,opened_by_user_id,reason_code,description,status) SELECT md5('seed:dispute:19')::uuid,d.id,d.customer_user_id,'WORK_DOES_NOT_MATCH_SCOPE','Нужна проверка соответствия результата согласованному объёму работ.','OPEN' FROM safe_deals d WHERE d.project_id=md5('seed:project:19')::uuid ON CONFLICT DO NOTHING;

-- ============================================================================
-- Economics demo: four COMPLETED Safe Deals, one per fee-payer model
-- (заказчик / исполнитель / 50-50 / платформа-субсидия). The AFTER INSERT
-- trigger on project_assignments creates each deal from the active rule; we
-- then rewrite the IMMUTABLE per-deal snapshot to the target model so all four
-- are visible locally. Sandbox provider cost is zero, so only the platform
-- commission is allocated. Work = 100 000 ₽, commission = 10% = 10 000 ₽; every
-- column is recomputed to satisfy the safe_deals dual-class invariants. Fully
-- idempotent via deterministic md5 ids + ON CONFLICT.
-- ============================================================================
DO $$
DECLARE
  modes text[]  := ARRAY['CUSTOMER','FREELANCER','SPLIT','PLATFORM'];
  titles text[] := ARRAY['Демо экономики: комиссию платит заказчик','Демо экономики: комиссию платит исполнитель','Демо экономики: комиссия пополам (50/50)','Демо экономики: комиссию берёт на себя платформа'];
  cust text[]   := ARRAY['customer@example.test','customer2@example.test','customer3@example.test','customer4@example.test'];
  free text[]   := ARRAY['go@example.test','flutter@example.test','fullstack@example.test','designer@example.test'];
  k int; w bigint; c bigint; share int; ver int; did uuid;
  pc bigint; pf bigint; pp bigint; ct bigint; fp bigint; sub bigint; net bigint;
  cu uuid; fu uuid; pid uuid; prop uuid; asg uuid;
BEGIN
  SELECT version INTO ver FROM safe_deal_fee_rules WHERE enabled ORDER BY effective_from DESC LIMIT 1;
  IF ver IS NULL THEN ver := 1; END IF;
  FOR k IN 1..4 LOOP
    w := 10000000;             -- 100 000 ₽ work amount
    c := (w*1000)/10000;       -- 10% platform commission = 10 000 ₽
    share := 5000;             -- customer pays 50% of the commission in SPLIT
    cu := md5('seed:user:'||cust[k])::uuid; fu := md5('seed:user:'||free[k])::uuid;
    pid := md5('seed:econ-project:'||k)::uuid; prop := md5('seed:econ-proposal:'||k)::uuid; asg := md5('seed:econ-assignment:'||k)::uuid;

    -- Allocate the commission by payer mode (provider cost is zero for sandbox).
    IF modes[k]='CUSTOMER' THEN pc:=c; pf:=0; pp:=0;
    ELSIF modes[k]='FREELANCER' THEN pc:=0; pf:=c; pp:=0;
    ELSIF modes[k]='PLATFORM' THEN pc:=0; pf:=0; pp:=c;
    ELSE pc:=(c*share)/10000; pf:=c-pc; pp:=0; END IF;
    ct := w + pc;    -- customer total: work + customer-borne commission
    fp := w - pf;    -- freelancer payout: work - freelancer-borne commission
    sub := pp;       -- subsidy = platform-borne commission (provider/discount/bonus all zero)
    net := c - sub;  -- net revenue = gross commission - subsidy

    INSERT INTO projects(id,customer_user_id,category_id,title,slug,description,budget_type,budget_min_kopecks,budget_max_kopecks,deadline_at,experience_level,visibility,status,source_type,published_at,proposal_count)
    VALUES(pid,cu,md5('seed:cat:development')::uuid,titles[k],'demo-econ-project-'||k,'Демонстрация распределения комиссии в безопасной сделке. Сделка завершена, экономика зафиксирована на момент принятия отклика и не меняется при смене правил.','FIXED',w,NULL,now()-(10+k)*interval '1 day','INTERMEDIATE','PUBLIC','COMPLETED','MANUAL',now()-(30+k)*interval '1 day',1)
    ON CONFLICT(id) DO UPDATE SET status='COMPLETED',title=EXCLUDED.title,visibility='PUBLIC';
    INSERT INTO proposals(id,project_id,freelancer_user_id,message,price_kopecks,delivery_days,status)
    VALUES(prop,pid,fu,'Отклик для демонстрации экономики безопасной сделки.',w,14,'ACCEPTED')
    ON CONFLICT(id) DO UPDATE SET status='ACCEPTED',price_kopecks=EXCLUDED.price_kopecks;
    -- Trigger project_assignment_safe_deal creates the deal from the active rule.
    INSERT INTO project_assignments(id,project_id,proposal_id,freelancer_user_id,agreed_price_kopecks,currency,status,started_at,completed_at)
    VALUES(asg,pid,prop,fu,w,'RUB','COMPLETED',now()-(9+k)*interval '1 day',now()-(2+k)*interval '1 day')
    ON CONFLICT(proposal_id) WHERE proposal_id IS NOT NULL DO NOTHING;

    -- Rewrite the immutable snapshot to the target payer model, then complete it.
    UPDATE safe_deals SET
      work_amount_kopecks=w, gross_amount_kopecks=ct, freelancer_amount_kopecks=fp, platform_fee_kopecks=c,
      platform_fee_customer_kopecks=pc, platform_fee_freelancer_kopecks=pf, platform_fee_platform_kopecks=pp,
      provider_fee_kopecks=0, provider_fee_customer_kopecks=0, provider_fee_freelancer_kopecks=0, provider_fee_platform_kopecks=0,
      customer_discount_kopecks=0, freelancer_bonus_kopecks=0,
      platform_provider_cost_kopecks=0, platform_subsidy_kopecks=sub, platform_net_revenue_kopecks=net,
      platform_fee_payer_mode=modes[k], platform_customer_share_basis_points=CASE WHEN modes[k]='SPLIT' THEN share ELSE 0 END,
      provider_fee_payer_mode='CUSTOMER', provider_customer_share_basis_points=0,
      fee_rule_version=ver, provider_pricing_version=1,
      status='COMPLETED', funded_at=now()-(8+k)*interval '1 day', work_started_at=now()-(7+k)*interval '1 day',
      submitted_at=now()-(3+k)*interval '1 day', accepted_at=now()-(2+k)*interval '1 day', completed_at=now()-(2+k)*interval '1 day', updated_at=now()-(2+k)*interval '1 day'
    WHERE project_id=pid;

    SELECT id INTO did FROM safe_deals WHERE project_id=pid LIMIT 1;
    INSERT INTO payment_records(id,deal_id,provider,provider_payment_id,provider_status,amount_kopecks,currency,idempotency_key)
    VALUES(md5('seed:econ-payment:'||k)::uuid,did,'sandbox','seed-econ-payment-'||k,'RELEASED',ct,'RUB','seed-econ-payment-'||k)
    ON CONFLICT(deal_id,idempotency_key) DO NOTHING;
  END LOOP;
END $$;

INSERT INTO external_reputations(id,user_id,platform,profile_url,external_username,rating,reviews_count,completed_orders_count,account_since,verification_status,verification_method,verified_at)
SELECT md5('seed:rep:'||i)::uuid,md5('seed:user:'||email)::uuid,platform,CASE platform WHEN 'KWORK' THEN 'https://kwork.ru/user/demo-'||i WHEN 'FL_RU' THEN 'https://www.fl.ru/users/demo-'||i WHEN 'GITHUB' THEN 'https://github.com/demo-'||i WHEN 'BEHANCE' THEN 'https://www.behance.net/demo-'||i WHEN 'HABR_CAREER' THEN 'https://career.habr.com/demo-'||i ELSE 'https://example.test/profile/'||i END,'demo-'||i,rating,reviews,orders,current_date-(600+i*70),status,CASE WHEN status='VERIFIED' THEN 'MANUAL' END,CASE WHEN status='VERIFIED' THEN now()-i*interval '10 day' END FROM (VALUES
 (1,'freelancer@example.test','KWORK',4.91,132,168,'VERIFIED'),(2,'flutter@example.test','HABR_CAREER',4.80,48,55,'VERIFIED'),(3,'go@example.test','GITHUB',4.95,76,91,'VERIFIED'),(4,'designer@example.test','BEHANCE',4.88,39,62,'VERIFIED'),(5,'ml@example.test','OTHER',4.70,18,24,'PENDING'),(6,'seo@example.test','FL_RU',4.92,104,139,'VERIFIED'),(7,'marketing@example.test','OTHER',4.60,26,31,'UNVERIFIED'),(8,'copywriter@example.test','OTHER',4.75,87,120,'REJECTED')) x(i,email,platform,rating,reviews,orders,status)
ON CONFLICT(user_id,platform,profile_url) DO UPDATE SET rating=EXCLUDED.rating,reviews_count=EXCLUDED.reviews_count,verification_status=EXCLUDED.verification_status;
-- Native trust stats are NEVER hardcoded: they are recomputed from real review
-- rows at the end of this file (see "Recompute trust") using the exact same
-- formula as reviews.recalculateTx, so every displayed rating traces to a
-- legitimate, completed-transaction review.
INSERT INTO reviews(id,project_id,reviewer_user_id,reviewee_user_id,reviewer_role,rating_overall,would_work_again,text,status)
SELECT md5('seed:review:20:c')::uuid,md5('seed:project:20')::uuid,p.customer_user_id,d.freelancer_user_id,'CUSTOMER',5,true,'Сильная работа: всё по этапам, понятная коммуникация и аккуратная передача результата.','PUBLISHED' FROM projects p JOIN safe_deals d ON d.project_id=p.id WHERE p.id=md5('seed:project:20')::uuid ON CONFLICT(project_id,reviewer_user_id) DO NOTHING;
INSERT INTO reviews(id,project_id,reviewer_user_id,reviewee_user_id,reviewer_role,rating_overall,would_work_again,text,status)
SELECT md5('seed:review:20:f')::uuid,md5('seed:project:20')::uuid,d.freelancer_user_id,p.customer_user_id,'FREELANCER',5,true,'Понятный бриф, быстрые ответы и оплата вовремя — комфортное сотрудничество.','PUBLISHED' FROM projects p JOIN safe_deals d ON d.project_id=p.id WHERE p.id=md5('seed:project:20')::uuid ON CONFLICT(project_id,reviewer_user_id) DO NOTHING;
INSERT INTO review_dimensions(review_id,dimension,score)
SELECT r.id,x.dimension,x.score FROM reviews r JOIN(VALUES('QUALITY',5),('DEADLINE',5),('COMMUNICATION',5),('BUDGET_ACCURACY',4))x(dimension,score) ON r.id=md5('seed:review:20:c')::uuid ON CONFLICT DO NOTHING;
INSERT INTO review_dimensions(review_id,dimension,score)
SELECT r.id,x.dimension,x.score FROM reviews r JOIN(VALUES('BRIEF_QUALITY',5),('COMMUNICATION',5),('PAYMENT_BEHAVIOR',5),('REASONABLE_REVISIONS',4))x(dimension,score) ON r.id=md5('seed:review:20:f')::uuid ON CONFLICT DO NOTHING;

-- ============================================================================
-- Reputation history: fully completed engagements that make BOTH sides eligible
-- to review each other. Each row = COMPLETED project + ACCEPTED proposal +
-- COMPLETED assignment + COMPLETED safe deal, then one CUSTOMER->FREELANCER and
-- one FREELANCER->CUSTOMER review with per-role dimensions. Distributed so
-- several freelancers AND several customers accumulate >=3 reviews (enough to
-- populate recommendation_rate and to inspect public profile review sections).
-- Deterministic md5 ids + ON CONFLICT keep the whole block idempotent.
-- ============================================================================
DO $$
DECLARE
  customers text[]:=ARRAY['customer@example.test','customer2@example.test','customer3@example.test','customer4@example.test','customer5@example.test','customer6@example.test'];
  freelancers text[]:=ARRAY['freelancer@example.test','flutter@example.test','go@example.test','fullstack@example.test','designer@example.test','ml@example.test','seo@example.test','marketing@example.test','copywriter@example.test','marketplace@example.test'];
  cidx int[]:=ARRAY[1,2,3,4, 1,2,3, 1,2,3,4, 1,2,5, 3,4,5, 6];
  fidx int[]:=ARRAY[1,1,1,1, 2,2,2, 3,3,3,3, 5,5,5, 7,7,7, 4];
  cust_over int[]:=ARRAY[5,5,4,5, 5,4,5, 5,4,5,4, 5,4,5, 5,4,5, 4];
  free_over int[]:=ARRAY[5,4,5,4, 5,5,4, 5,5,4,5, 4,5,5, 4,5,5, 5];
  cust_again boolean[]:=ARRAY[true,true,false,true, true,true,true, true,false,true,true, true,true,true, true,false,true, true];
  free_again boolean[]:=ARRAY[true,true,true,true, true,true,true, true,false,true,true, true,true,true, false,true,true, true];
  titles text[]:=ARRAY['Telegram-бот для приёма заявок','Автоматизация продаж в Telegram','Чат-бот поддержки клиентов','Интеграция Telegram-бота с CRM','Мобильное приложение доставки','Flutter-приложение лояльности','Обновление мобильного каталога','Go API для личного кабинета','Микросервис расчётов на Go','Бэкенд платёжного модуля','Оптимизация Go-сервиса отчётов','Редизайн B2B-платформы','Дизайн-система продукта','UX-аудит и прототип кабинета','SEO-аудит интернет-магазина','Техническая оптимизация сайта','Контент-стратегия для блога','MVP на Next.js и Go'];
  cust_texts text[]:=ARRAY['Отличная работа: всё по этапам, держал в курсе и сдал в срок. Рекомендую.','Аккуратно разобрался в задаче, предложил улучшения и довёл до результата без лишних правок.','Сильная экспертиза и понятная коммуникация. Результат приняли без замечаний.','Работу выполнил качественно, но пару раз пришлось уточнять детали по срокам.','Профессиональный подход: прозрачная оценка, промежуточные демо и чистая передача материалов.','Хороший специалист: всё задокументировал и объяснил, как поддерживать решение дальше.'];
  free_texts text[]:=ARRAY['Заказчик дал понятный бриф и оперативно отвечал на вопросы. Оплата вовремя.','Комфортное сотрудничество: адекватные правки, чёткие приоритеты и своевременная оплата.','Хорошо сформулированная задача и конструктивная обратная связь на каждом этапе.','Правки были в рамках согласованного объёма, коммуникация ровная. Готов работать снова.','Заказчик вовлечён в процесс, быстро принимал решения и подтверждал этапы.','Бриф местами приходилось уточнять, но в целом сотрудничество прошло продуктивно.'];
  k int; d int; cu uuid; fu uuid; pid uuid; prop uuid; asg uuid; price bigint; cat text; skill text;
  cdim text[]:=ARRAY['QUALITY','DEADLINE','COMMUNICATION','BUDGET_ACCURACY'];
  fdim text[]:=ARRAY['BRIEF_QUALITY','COMMUNICATION','PAYMENT_BEHAVIOR','REASONABLE_REVISIONS'];
  crid uuid; frid uuid;
BEGIN
  FOR k IN 1..array_length(cidx,1) LOOP
    cu:=md5('seed:user:'||customers[cidx[k]])::uuid;
    fu:=md5('seed:user:'||freelancers[fidx[k]])::uuid;
    pid:=md5('seed:rep-project:'||k)::uuid;
    prop:=md5('seed:rep-proposal:'||k)::uuid;
    asg:=md5('seed:rep-assignment:'||k)::uuid;
    price:=1800000+k*90000;
    cat:=CASE fidx[k] WHEN 5 THEN 'design' WHEN 7 THEN 'seo' ELSE 'development' END;
    skill:=CASE fidx[k] WHEN 1 THEN 'telegram' WHEN 2 THEN 'flutter' WHEN 3 THEN 'go' WHEN 4 THEN 'nextjs' WHEN 5 THEN 'figma' WHEN 7 THEN 'seo' ELSE 'typescript' END;
    crid:=md5('seed:rep-review:'||k||':c')::uuid;
    frid:=md5('seed:rep-review:'||k||':f')::uuid;

    INSERT INTO projects(id,customer_user_id,category_id,title,slug,description,budget_type,budget_min_kopecks,budget_max_kopecks,deadline_at,experience_level,visibility,status,source_type,published_at,proposal_count)
    VALUES(pid,cu,md5('seed:cat:'||cat)::uuid,titles[k],'demo-rep-project-'||k,'Проект завершён через безопасную сделку. Ниже — публичные отзывы обеих сторон по результатам сотрудничества.','RANGE',price-200000,price+400000,now()-(20+k)*interval '1 day','INTERMEDIATE','PUBLIC','COMPLETED','MANUAL',now()-(45+k)*interval '1 day',1)
    ON CONFLICT(id) DO UPDATE SET status='COMPLETED',customer_user_id=EXCLUDED.customer_user_id,title=EXCLUDED.title,published_at=EXCLUDED.published_at,visibility='PUBLIC';
    INSERT INTO project_skills(project_id,skill_id,importance) VALUES(pid,md5('seed:skill:'||skill)::uuid,100) ON CONFLICT DO NOTHING;

    INSERT INTO proposals(id,project_id,freelancer_user_id,message,price_kopecks,delivery_days,status)
    VALUES(prop,pid,fu,'Финальный отклик по завершённому проекту: согласованный объём, план по этапам и передача результатов.',price,10+k%10,'ACCEPTED')
    ON CONFLICT(id) DO UPDATE SET status='ACCEPTED',price_kopecks=EXCLUDED.price_kopecks;

    INSERT INTO project_assignments(id,project_id,proposal_id,freelancer_user_id,agreed_price_kopecks,currency,status,started_at,completed_at)
    VALUES(asg,pid,prop,fu,price,'RUB','COMPLETED',now()-(18+k)*interval '1 day',now()-(3+k%5)*interval '1 day')
    ON CONFLICT(proposal_id) WHERE proposal_id IS NOT NULL DO NOTHING;

    UPDATE safe_deals SET status='COMPLETED',funded_at=now()-(17+k)*interval '1 day',work_started_at=now()-(16+k)*interval '1 day',submitted_at=now()-(6+k%5)*interval '1 day',accepted_at=now()-(4+k%5)*interval '1 day',completed_at=now()-(3+k%5)*interval '1 day',updated_at=now()-(3+k%5)*interval '1 day' WHERE project_id=pid;

    INSERT INTO reviews(id,project_id,reviewer_user_id,reviewee_user_id,reviewer_role,rating_overall,would_work_again,text,status,created_at,updated_at)
    VALUES(crid,pid,cu,fu,'CUSTOMER',cust_over[k],cust_again[k],cust_texts[1+(k-1)%6],'PUBLISHED',now()-(3+k%6)*interval '1 day',now()-(3+k%6)*interval '1 day')
    ON CONFLICT(project_id,reviewer_user_id) DO UPDATE SET rating_overall=EXCLUDED.rating_overall,would_work_again=EXCLUDED.would_work_again,text=EXCLUDED.text,status='PUBLISHED';
    INSERT INTO reviews(id,project_id,reviewer_user_id,reviewee_user_id,reviewer_role,rating_overall,would_work_again,text,status,created_at,updated_at)
    VALUES(frid,pid,fu,cu,'FREELANCER',free_over[k],free_again[k],free_texts[1+(k-1)%6],'PUBLISHED',now()-(2+k%6)*interval '1 day',now()-(2+k%6)*interval '1 day')
    ON CONFLICT(project_id,reviewer_user_id) DO UPDATE SET rating_overall=EXCLUDED.rating_overall,would_work_again=EXCLUDED.would_work_again,text=EXCLUDED.text,status='PUBLISHED';

    FOR d IN 1..4 LOOP
      INSERT INTO review_dimensions(review_id,dimension,score)
      SELECT r.id,cdim[d],greatest(1,least(5,cust_over[k]-(CASE WHEN (k+d)%3=0 THEN 1 ELSE 0 END))) FROM reviews r WHERE r.project_id=pid AND r.reviewer_user_id=cu ON CONFLICT DO NOTHING;
      INSERT INTO review_dimensions(review_id,dimension,score)
      SELECT r.id,fdim[d],greatest(1,least(5,free_over[k]-(CASE WHEN (k+d)%3=1 THEN 1 ELSE 0 END))) FROM reviews r WHERE r.project_id=pid AND r.reviewer_user_id=fu ON CONFLICT DO NOTHING;
    END LOOP;
  END LOOP;
END $$;

-- A few in-app "new review received" notifications so the notification side of
-- the review lifecycle is visible locally (the worker normally emits these from
-- REVIEW_CREATED outbox events).
INSERT INTO notifications(id,dedupe_key,user_id,type,actor_user_id,entity_type,entity_id,payload,read_at,created_at)
SELECT md5('seed:notif:rep-review:'||k)::uuid,'seed-rep-review-'||k,r.reviewee_user_id,'new_review_received',r.reviewer_user_id,'event',r.id,jsonb_build_object('event_type','REVIEW_CREATED','entity_id',r.id::text),CASE WHEN k%2=0 THEN now()-interval '2 days' ELSE NULL END,now()-(3+k%5)*interval '1 day'
FROM generate_series(1,8)k JOIN reviews r ON r.id=md5('seed:rep-review:'||k||':c')::uuid
ON CONFLICT(dedupe_key) DO NOTHING;

-- Recompute trust: native_rating / reviews_count / completed_projects_count /
-- recommendation_rate are derived ONLY from real review rows and completed
-- projects, using the identical formula as reviews.recalculateTx. Every seeded
-- demo user is covered so no stale/fake rating can survive a re-seed; users
-- without reviews get NULL rating + 0 reviews (never a fabricated score).
-- completion_rate / on_time_rate / repeat_rate / avg_response_minutes are left
-- unset (NULL) rather than fabricated — the matching scorer treats them as
-- absent signals.
INSERT INTO user_trust_stats(user_id,native_rating,reviews_count,completed_projects_count,recommendation_rate,updated_at)
SELECT u.id,
  round(avg(r.rating_overall) FILTER(WHERE r.status='PUBLISHED'),2),
  count(r.id) FILTER(WHERE r.status='PUBLISHED'),
  (SELECT count(DISTINCT p.id) FROM projects p LEFT JOIN project_assignments a ON a.project_id=p.id WHERE p.status='COMPLETED' AND(p.customer_user_id=u.id OR a.freelancer_user_id=u.id)),
  CASE WHEN count(r.would_work_again) FILTER(WHERE r.status='PUBLISHED')>=3 THEN round(100.0*count(*) FILTER(WHERE r.status='PUBLISHED' AND r.would_work_again=true)/NULLIF(count(r.would_work_again) FILTER(WHERE r.status='PUBLISHED'),0),2) END,
  now()
FROM users u LEFT JOIN reviews r ON r.reviewee_user_id=u.id
WHERE u.email_normalized LIKE '%@example.test'
GROUP BY u.id
ON CONFLICT(user_id) DO UPDATE SET native_rating=EXCLUDED.native_rating,reviews_count=EXCLUDED.reviews_count,completed_projects_count=EXCLUDED.completed_projects_count,recommendation_rate=EXCLUDED.recommendation_rate,updated_at=EXCLUDED.updated_at;

INSERT INTO conversations(id,kind,project_id,updated_at) SELECT md5('seed:conversation:'||i)::uuid,'PROJECT',md5('seed:project:'||i)::uuid,now()-i*interval '1 hour' FROM generate_series(15,20)i ON CONFLICT(project_id) WHERE project_id IS NOT NULL DO NOTHING;
INSERT INTO conversation_members(conversation_id,user_id) SELECT c.id,p.customer_user_id FROM conversations c JOIN projects p ON p.id=c.project_id WHERE p.slug LIKE 'demo-project-%' ON CONFLICT DO NOTHING;
INSERT INTO conversation_members(conversation_id,user_id) SELECT c.id,a.freelancer_user_id FROM conversations c JOIN project_assignments a ON a.project_id=c.project_id WHERE c.project_id IS NOT NULL ON CONFLICT DO NOTHING;
INSERT INTO messages(id,conversation_id,sender_user_id,type,body,client_message_id,created_at)
SELECT md5('seed:message:'||i||':'||n)::uuid,c.id,CASE WHEN n%2=1 THEN p.customer_user_id ELSE a.freelancer_user_id END,'TEXT',CASE n WHEN 1 THEN 'Здравствуйте! Уточнил контекст проекта в описании.' WHEN 2 THEN 'Спасибо, вижу. Начну с согласованного первого этапа.' ELSE 'Хорошо, буду ждать промежуточный результат.' END,md5('seed:client-message:'||i||':'||n)::uuid,now()-(10-n)*interval '1 hour' FROM generate_series(15,20)i CROSS JOIN generate_series(1,3)n JOIN conversations c ON c.project_id=md5('seed:project:'||i)::uuid JOIN projects p ON p.id=c.project_id JOIN project_assignments a ON a.project_id=p.id ON CONFLICT(sender_user_id,client_message_id) WHERE client_message_id IS NOT NULL DO NOTHING;
INSERT INTO notifications(id,dedupe_key,user_id,type,entity_type,entity_id,payload,read_at,created_at)
SELECT md5('seed:notification:'||i)::uuid,'seed-notification-'||i,md5('seed:user:customer@example.test')::uuid,CASE WHEN i%2=0 THEN 'proposal.created' ELSE 'message.created' END,'PROJECT',md5('seed:project:'||(1+i%14))::uuid,jsonb_build_object('title','Обновление по проекту'),CASE WHEN i<=3 THEN NULL ELSE now()-interval '1 day' END,now()-i*interval '2 hour' FROM generate_series(1,8)i ON CONFLICT(dedupe_key) DO NOTHING;
INSERT INTO customer_team_members(customer_user_id,freelancer_user_id,label,notes) VALUES(md5('seed:user:customer@example.test')::uuid,md5('seed:user:freelancer@example.test')::uuid,'Telegram и автоматизация','Проверенный специалист для быстрых запусков'),(md5('seed:user:customer@example.test')::uuid,md5('seed:user:designer@example.test')::uuid,'Дизайн продукта','UX и дизайн-системы') ON CONFLICT DO NOTHING;
INSERT INTO favorites(user_id,entity_type,entity_id) VALUES(md5('seed:user:customer@example.test')::uuid,'FREELANCER',md5('seed:user:go@example.test')::uuid),(md5('seed:user:customer@example.test')::uuid,'SERVICE',md5('seed:service:4')::uuid),(md5('seed:user:freelancer@example.test')::uuid,'PROJECT',md5('seed:project:3')::uuid) ON CONFLICT DO NOTHING;

INSERT INTO companies(id,owner_user_id,name,slug,website,description,verification_status) VALUES
 (md5('seed:company:1')::uuid,md5('seed:user:customer@example.test')::uuid,'Север Диджитал','sever-digital','https://example.test','Продуктовая команда для B2B-сервисов.','VERIFIED'),
 (md5('seed:company:2')::uuid,md5('seed:user:customer2@example.test')::uuid,'Орбита Тех','orbita-tech','https://example.test','Разрабатываем инструменты для электронной коммерции.','VERIFIED'),
 (md5('seed:company:3')::uuid,md5('seed:user:customer3@example.test')::uuid,'Линия роста','growth-line','https://example.test','Маркетинговые продукты и аналитика.','UNVERIFIED') ON CONFLICT(owner_user_id,slug) DO UPDATE SET name=EXCLUDED.name;
DO $$ DECLARE titles text[]:=ARRAY['Senior Go разработчик','Flutter разработчик','Продуктовый дизайнер','Frontend разработчик React','ML Engineer','SEO-специалист','Performance-маркетолог','Редактор B2B-контента','Менеджер маркетплейсов','Backend разработчик']; i int; BEGIN FOR i IN 1..10 LOOP INSERT INTO jobs(id,company_id,customer_user_id,category_id,title,slug,description,employment_type,salary_min_kopecks,salary_max_kopecks,location_text,remote,experience_level,status,published_at) VALUES(md5('seed:job:'||i)::uuid,md5('seed:company:'||(1+(i-1)%3))::uuid,md5('seed:user:customer'||CASE WHEN 1+(i-1)%3=1 THEN '' ELSE (1+(i-1)%3)::text END||'@example.test')::uuid,md5('seed:cat:'||CASE WHEN i IN(3) THEN 'design' WHEN i IN(5) THEN 'ai' WHEN i IN(6) THEN 'seo' WHEN i IN(7) THEN 'marketing' WHEN i IN(8) THEN 'content' WHEN i IN(9) THEN 'marketplaces' ELSE 'development' END)::uuid,titles[i],'demo-vacancy-'||i,'Ищем специалиста в продуктовую команду. Важны самостоятельность, понятная коммуникация и опыт запуска реальных задач.','FULL_TIME',18000000+i*1200000,26000000+i*1800000,CASE WHEN i%2=0 THEN 'Москва' ELSE 'Россия' END,true,CASE WHEN i%3=0 THEN 'SENIOR' ELSE 'MIDDLE' END,'PUBLISHED',now()-i*interval '1 day') ON CONFLICT(id) DO UPDATE SET title=EXCLUDED.title,status='PUBLISHED',published_at=EXCLUDED.published_at; END LOOP; END $$;

-- Admin / moderation QA fixtures. Keep deterministic and development-only.
INSERT INTO users(id,email,email_normalized,password_hash,username,username_normalized,display_name,email_verified_at,status)
VALUES
 (md5('seed:user:superadmin@example.test')::uuid,'superadmin@example.test','superadmin@example.test',current_setting('freelance.seed_hash'),'superadmin-demo','superadmin-demo','Светлана Суперадмин',now(),'ACTIVE'),
 (md5('seed:user:suspended@example.test')::uuid,'suspended@example.test','suspended@example.test',current_setting('freelance.seed_hash'),'suspended-demo','suspended-demo','Тест Приостановлен',now(),'SUSPENDED'),
 (md5('seed:user:banned@example.test')::uuid,'banned@example.test','banned@example.test',current_setting('freelance.seed_hash'),'banned-demo','banned-demo','Тест Заблокирован',now(),'BANNED')
ON CONFLICT(email_normalized) WHERE deleted_at IS NULL DO UPDATE SET password_hash=EXCLUDED.password_hash,display_name=EXCLUDED.display_name,status=EXCLUDED.status;
INSERT INTO user_roles(user_id,role,granted_by) VALUES
 (md5('seed:user:superadmin@example.test')::uuid,'SUPER_ADMIN',md5('seed:user:superadmin@example.test')::uuid),
 (md5('seed:user:superadmin@example.test')::uuid,'ADMIN',md5('seed:user:superadmin@example.test')::uuid)
ON CONFLICT DO NOTHING;
DELETE FROM user_capabilities WHERE user_id=md5('seed:user:superadmin@example.test')::uuid;

INSERT INTO feature_flags(key,enabled,config,description,updated_by,updated_at) VALUES
 ('safe_deal',true,'{"sandbox_only":true}'::jsonb,'Обязательная безопасная сделка для оплачиваемых проектов',md5('seed:user:admin@example.test')::uuid,now()),
 ('ai_project_builder',true,'{"anonymous_daily_limit":5}'::jsonb,'AI-помощник создания проектного брифа',md5('seed:user:admin@example.test')::uuid,now()),
 ('ai_matching',true,'{"llm_rerank":false}'::jsonb,'Детерминированный Smart Match и опциональный AI rerank',md5('seed:user:admin@example.test')::uuid,now()),
 ('jobs',true,'{}'::jsonb,'Вакансии и отклики',md5('seed:user:admin@example.test')::uuid,now()),
 ('education',true,'{}'::jsonb,'Обучение и наставничество как типы услуг',md5('seed:user:admin@example.test')::uuid,now()),
 ('pro_subscriptions_enabled',true,'{}'::jsonb,'Публичные PRO-поверхности и применение PRO-привилегий',md5('seed:user:admin@example.test')::uuid,now()),
 ('blog_enabled',true,'{}'::jsonb,'Публичный блог и его навигация',md5('seed:user:admin@example.test')::uuid,now())
ON CONFLICT(key) DO UPDATE SET enabled=EXCLUDED.enabled,config=EXCLUDED.config,description=EXCLUDED.description,updated_by=EXCLUDED.updated_by,updated_at=EXCLUDED.updated_at;

UPDATE external_reputations SET verification_method='MANUAL', evidence=jsonb_build_object('note','Демонстрационная ручная проверка','screenshot_ref','dev-evidence/ml-profile.png'), updated_at=now()
WHERE id=md5('seed:rep:5')::uuid AND verification_status='PENDING';
INSERT INTO reputation_verification_challenges(id,external_reputation_id,method,expires_at,attempts,status)
VALUES(md5('seed:rep-challenge:5')::uuid,md5('seed:rep:5')::uuid,'MANUAL',now()+interval '7 days',0,'PENDING')
ON CONFLICT(id) DO UPDATE SET expires_at=EXCLUDED.expires_at,status='PENDING';

INSERT INTO reports(id,reporter_user_id,entity_type,entity_id,reason_code,description,status,created_at) VALUES
 (md5('seed:report:project')::uuid,md5('seed:user:freelancer@example.test')::uuid,'PROJECT',md5('seed:project:6')::uuid,'SUSPICIOUS_SCOPE','Описание проекта содержит спорные условия — нужна проверка модератора.','OPEN',now()-interval '3 hours'),
 (md5('seed:report:service')::uuid,md5('seed:user:customer@example.test')::uuid,'SERVICE',md5('seed:service:8')::uuid,'MISLEADING','В карточке услуги заявлены результаты, которые нужно подтвердить.','IN_REVIEW',now()-interval '9 hours'),
 (md5('seed:report:review')::uuid,md5('seed:user:freelancer@example.test')::uuid,'REVIEW',md5('seed:review:20:c')::uuid,'PERSONAL_DATA','Проверьте отзыв на наличие лишних персональных данных.','OPEN',now()-interval '1 day')
ON CONFLICT(id) DO UPDATE SET status=EXCLUDED.status,description=EXCLUDED.description;

INSERT INTO fraud_signals(id,user_id,entity_type,entity_id,signal_type,severity,evidence,status,created_at) VALUES
 (md5('seed:fraud:velocity')::uuid,md5('seed:user:marketing@example.test')::uuid,'USER',md5('seed:user:marketing@example.test')::uuid,'PROPOSAL_VELOCITY',3,'{"window":"15m","count":18,"note":"dev fixture"}'::jsonb,'OPEN',now()-interval '45 minutes'),
 (md5('seed:fraud:duplicate')::uuid,md5('seed:user:copywriter@example.test')::uuid,'PROJECT',md5('seed:project:8')::uuid,'DUPLICATE_CONTENT',2,'{"similarity":0.93,"note":"dev fixture"}'::jsonb,'REVIEWING',now()-interval '6 hours'),
 (md5('seed:fraud:referral')::uuid,md5('seed:user:customer4@example.test')::uuid,'USER',md5('seed:user:customer4@example.test')::uuid,'REFERRAL_PATTERN',4,'{"accounts":4,"note":"manual review required"}'::jsonb,'OPEN',now()-interval '1 day')
ON CONFLICT(id) DO UPDATE SET status=EXCLUDED.status,evidence=EXCLUDED.evidence;

INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,created_at) VALUES
 (md5('seed:audit:1')::uuid,md5('seed:user:admin@example.test')::uuid,'feature_flag.updated','FEATURE_FLAG',NULL,'{"key":"safe_deal","reason":"dev fixture"}'::jsonb,now()-interval '30 minutes'),
 (md5('seed:audit:2')::uuid,md5('seed:user:moderator@example.test')::uuid,'reputation.reviewed','EXTERNAL_REPUTATION',md5('seed:rep:1')::uuid,'{"decision":"VERIFIED","reason":"dev fixture"}'::jsonb,now()-interval '4 hours'),
 (md5('seed:audit:3')::uuid,md5('seed:user:admin@example.test')::uuid,'user.sessions_revoked','USER',md5('seed:user:customer2@example.test')::uuid,'{"reason":"dev security exercise"}'::jsonb,now()-interval '1 day')
ON CONFLICT(id) DO NOTHING;

-- Extra deterministic product journeys for manual local QA.
INSERT INTO job_applications(id,job_id,user_id,cover_message,status,created_at,updated_at) VALUES
 (md5('seed:job-app:1')::uuid,md5('seed:job:1')::uuid,md5('seed:user:go@example.test')::uuid,'Есть опыт проектирования Go API и PostgreSQL под продуктовую нагрузку. Готов обсудить архитектуру и этапы.','SHORTLISTED',now()-interval '18 hours',now()-interval '6 hours'),
 (md5('seed:job-app:2')::uuid,md5('seed:job:2')::uuid,md5('seed:user:flutter@example.test')::uuid,'Разрабатываю Flutter-приложения и могу показать релевантные кейсы.','VIEWED',now()-interval '13 hours',now()-interval '4 hours'),
 (md5('seed:job-app:3')::uuid,md5('seed:job:3')::uuid,md5('seed:user:designer@example.test')::uuid,'Работаю с продуктовыми интерфейсами, исследованиями и дизайн-системами.','SUBMITTED',now()-interval '7 hours',now()-interval '7 hours')
ON CONFLICT(job_id,user_id) DO UPDATE SET cover_message=EXCLUDED.cover_message,status=EXCLUDED.status,updated_at=EXCLUDED.updated_at;

INSERT INTO referral_rules(id,code,event_type,beneficiary,reward_type,reward_value,reward_unit,max_uses,enabled,config) VALUES
 (md5('seed:ref-rule:welcome')::uuid,'WELCOME_INVITER','INVITE_ACCEPTED','INVITER','BONUS',5,'CREDITS',1000,true,'{"valid_days":90}'::jsonb)
ON CONFLICT(code) DO UPDATE SET enabled=true,reward_value=EXCLUDED.reward_value,config=EXCLUDED.config,updated_at=now();

INSERT INTO invites(id,inviter_user_id,invite_type,project_id,token_hash,intended_email,expires_at,accepted_by_user_id,accepted_at,created_at) VALUES
 (md5('seed:invite:project')::uuid,md5('seed:user:customer@example.test')::uuid,'PROJECT',md5('seed:project:4')::uuid,digest('demo-project-invite','sha256'),'flutter@example.test',now()+interval '30 days',md5('seed:user:flutter@example.test')::uuid,now()-interval '2 days',now()-interval '3 days'),
 (md5('seed:invite:pending')::uuid,md5('seed:user:freelancer@example.test')::uuid,'CUSTOMER',NULL,digest('demo-customer-invite','sha256'),'new-customer@example.test',now()+interval '30 days',NULL,NULL,now()-interval '1 day')
ON CONFLICT(id) DO UPDATE SET expires_at=EXCLUDED.expires_at,accepted_by_user_id=EXCLUDED.accepted_by_user_id,accepted_at=EXCLUDED.accepted_at;

INSERT INTO project_invited_users(project_id,user_id,invite_id,invited_role,accepted_at) VALUES
 (md5('seed:project:4')::uuid,md5('seed:user:flutter@example.test')::uuid,md5('seed:invite:project')::uuid,'FREELANCER',now()-interval '2 days')
ON CONFLICT(project_id,user_id) DO NOTHING;

INSERT INTO referral_attributions(id,inviter_user_id,invited_user_id,invite_id,first_touch_at,source) VALUES
 (md5('seed:ref-attr:1')::uuid,md5('seed:user:customer@example.test')::uuid,md5('seed:user:flutter@example.test')::uuid,md5('seed:invite:project')::uuid,now()-interval '3 days','project_invite')
ON CONFLICT(invited_user_id) DO NOTHING;

INSERT INTO reward_ledger(id,user_id,rule_id,event_key,reward_type,amount,unit,expires_at) VALUES
 (md5('seed:reward:1')::uuid,md5('seed:user:customer@example.test')::uuid,md5('seed:ref-rule:welcome')::uuid,'seed:invite:project:accepted','BONUS',5,'CREDITS',now()+interval '90 days')
ON CONFLICT(user_id,event_key) DO NOTHING;

INSERT INTO notifications(id,dedupe_key,user_id,type,entity_type,entity_id,payload,read_at,created_at) VALUES
 (md5('seed:notification:freelancer:1')::uuid,'seed-notification-freelancer-1',md5('seed:user:freelancer@example.test')::uuid,'proposal.accepted','PROJECT',md5('seed:project:15')::uuid,'{"title":"Ваш отклик принят"}'::jsonb,NULL,now()-interval '50 minutes'),
 (md5('seed:notification:admin:1')::uuid,'seed-notification-admin-1',md5('seed:user:admin@example.test')::uuid,'moderation.queue','REPORT',md5('seed:report:project')::uuid,'{"title":"Новая жалоба требует проверки"}'::jsonb,NULL,now()-interval '20 minutes')
ON CONFLICT(dedupe_key) DO NOTHING;

-- Phase 9: configurable FREE/PRO plans, lifecycle examples and editorial CMS.
INSERT INTO subscription_plans(id,code,name,description,tier,billing_period,currency,amount_kopecks,active,display_order) VALUES
 (md5('seed:plan:free')::uuid,'free','Бесплатный','Все основные возможности маркетплейса','FREE','NONE','RUB',0,true,0),
 (md5('seed:plan:pro-month')::uuid,'pro_month','PRO на месяц','Больше портфолио, аналитика и приоритетная видимость','PRO','MONTH','RUB',99000,true,10),
 (md5('seed:plan:pro-year')::uuid,'pro_year','PRO на год','Годовой доступ ко всем преимуществам PRO','PRO','YEAR','RUB',990000,true,20)
ON CONFLICT(code) DO UPDATE SET name=EXCLUDED.name,description=EXCLUDED.description,amount_kopecks=EXCLUDED.amount_kopecks,active=EXCLUDED.active,display_order=EXCLUDED.display_order,updated_at=now();

INSERT INTO subscription_plan_entitlements(plan_id,feature_key,kind,enabled,limit_value,unlimited,config)
SELECT p.id,v.feature_key,v.kind,v.enabled,v.limit_value,v.unlimited,v.config FROM subscription_plans p JOIN (VALUES
 ('free','profile.pro_badge','BOOLEAN',false,NULL::bigint,false,'{}'::jsonb),
 ('free','profile.analytics','BOOLEAN',false,NULL::bigint,false,'{}'::jsonb),
 ('free','search.priority_visibility','BOOLEAN',false,NULL::bigint,false,'{}'::jsonb),
 ('free','portfolio.item_limit','LIMIT',true,8::bigint,false,'{}'::jsonb),
 ('free','portfolio.media_limit','LIMIT',true,8::bigint,false,'{}'::jsonb),
 ('pro_month','profile.pro_badge','BOOLEAN',true,NULL::bigint,false,'{}'::jsonb),
 ('pro_month','profile.analytics','BOOLEAN',true,NULL::bigint,false,'{}'::jsonb),
 ('pro_month','search.priority_visibility','BOOLEAN',true,NULL::bigint,false,'{"ranking_multiplier":1.08}'::jsonb),
 ('pro_month','portfolio.item_limit','LIMIT',true,40::bigint,false,'{}'::jsonb),
 ('pro_month','portfolio.media_limit','LIMIT',true,20::bigint,false,'{}'::jsonb),
 ('pro_year','profile.pro_badge','BOOLEAN',true,NULL::bigint,false,'{}'::jsonb),
 ('pro_year','profile.analytics','BOOLEAN',true,NULL::bigint,false,'{}'::jsonb),
 ('pro_year','search.priority_visibility','BOOLEAN',true,NULL::bigint,false,'{"ranking_multiplier":1.08}'::jsonb),
 ('pro_year','portfolio.item_limit','LIMIT',true,40::bigint,false,'{}'::jsonb),
 ('pro_year','portfolio.media_limit','LIMIT',true,20::bigint,false,'{}'::jsonb)
)v(plan_code,feature_key,kind,enabled,limit_value,unlimited,config) ON p.code=v.plan_code
ON CONFLICT(plan_id,feature_key) DO UPDATE SET kind=EXCLUDED.kind,enabled=EXCLUDED.enabled,limit_value=EXCLUDED.limit_value,unlimited=EXCLUDED.unlimited,config=EXCLUDED.config,updated_at=now();

INSERT INTO user_subscriptions(id,user_id,plan_id,status,starts_at,current_period_start,current_period_end,created_at,updated_at) VALUES
 (md5('seed:subscription:active')::uuid,md5('seed:user:fullstack@example.test')::uuid,md5('seed:plan:pro-month')::uuid,'ACTIVE',now()-interval '5 days',now()-interval '5 days',now()+interval '25 days',now()-interval '5 days',now()),
 (md5('seed:subscription:expired')::uuid,md5('seed:user:seo@example.test')::uuid,md5('seed:plan:pro-year')::uuid,'EXPIRED',now()-interval '400 days',now()-interval '400 days',now()-interval '35 days',now()-interval '400 days',now())
ON CONFLICT(id) DO UPDATE SET plan_id=EXCLUDED.plan_id,status=EXCLUDED.status,starts_at=EXCLUDED.starts_at,current_period_start=EXCLUDED.current_period_start,current_period_end=EXCLUDED.current_period_end,updated_at=now();
INSERT INTO subscription_events(id,subscription_id,event_type,to_status,actor_user_id,reason,created_at) VALUES
 (md5('seed:subscription-event:active')::uuid,md5('seed:subscription:active')::uuid,'ADMIN_GRANTED','ACTIVE',md5('seed:user:admin@example.test')::uuid,'Демонстрационный доступ для проверки PRO',now()-interval '5 days'),
 (md5('seed:subscription-event:expired')::uuid,md5('seed:subscription:expired')::uuid,'ADMIN_EXPIRED','EXPIRED',md5('seed:user:admin@example.test')::uuid,'Демонстрация истории истёкшей подписки',now()-interval '35 days')
ON CONFLICT(id) DO NOTHING;

INSERT INTO blog_categories(id,name,slug,description) VALUES
 (md5('seed:blog-category:work')::uuid,'Работа и карьера','work','Практика работы на фрилансе и развитие карьеры'),
 (md5('seed:blog-category:business')::uuid,'Бизнес','business','Как ставить задачи и работать со специалистами'),
 (md5('seed:blog-category:product')::uuid,'Продукт','product','Дизайн, разработка и запуск цифровых продуктов')
ON CONFLICT(slug) DO UPDATE SET name=EXCLUDED.name,description=EXCLUDED.description,updated_at=now();
INSERT INTO blog_tags(id,name,slug) VALUES
 (md5('seed:blog-tag:brief')::uuid,'Бриф','brief'),(md5('seed:blog-tag:portfolio')::uuid,'Портфолио','portfolio'),(md5('seed:blog-tag:safe-deal')::uuid,'Safe Deal','safe-deal')
ON CONFLICT(slug) DO UPDATE SET name=EXCLUDED.name,updated_at=now();
INSERT INTO blog_posts(id,author_user_id,category_id,title,slug,excerpt,content_html,status,seo_title,seo_description,published_at,scheduled_at,created_at,updated_at) VALUES
 (md5('seed:blog-post:brief')::uuid,md5('seed:user:admin@example.test')::uuid,md5('seed:blog-category:business')::uuid,'Как составить бриф, на который хочется откликнуться','kak-sostavit-brief','Понятная структура задачи помогает быстрее найти подходящего специалиста и точнее оценить сроки.','<h2>Начните с результата</h2><p>Опишите, что должно измениться для бизнеса или пользователя после завершения работы.</p><h2>Добавьте контекст</h2><ul><li>Для кого создаётся продукт</li><li>Что уже готово</li><li>Какие ограничения важно учесть</li></ul><blockquote>Хороший бриф оставляет пространство для экспертизы, но не скрывает критерии приёмки.</blockquote>','PUBLISHED','Как составить бриф для фрилансера','Практическая структура брифа для быстрой и точной оценки проекта.',now()-interval '8 days',NULL,now()-interval '9 days',now()-interval '8 days'),
 (md5('seed:blog-post:portfolio')::uuid,md5('seed:user:admin@example.test')::uuid,md5('seed:blog-category:work')::uuid,'Портфолио, которое объясняет ценность вашей работы','portfolio-kotoroe-rabotaet','Разбираем, как превратить набор картинок в убедительные продуктовые кейсы.','<h2>Покажите задачу</h2><p>Клиенту важно понять не только что вы сделали, но и почему выбрали именно это решение.</p><h2>Зафиксируйте вклад</h2><p>Отделяйте личную работу от результата всей команды и добавляйте измеримые факты.</p>','PUBLISHED','Как оформить портфолио специалиста','Структура сильного кейса: задача, решения, личный вклад и результат.',now()-interval '4 days',NULL,now()-interval '5 days',now()-interval '4 days'),
 (md5('seed:blog-post:safe-deal')::uuid,md5('seed:user:admin@example.test')::uuid,md5('seed:blog-category:product')::uuid,'Зачем фиксировать этапы работы до старта','zachem-fiksirovat-etapy','Этапы, критерии приёмки и прозрачная коммуникация снижают риски для обеих сторон.','<h2>Один результат — несколько проверок</h2><p>Разбейте большую задачу на наблюдаемые промежуточные результаты.</p><h2>Согласуйте изменения</h2><p>Заранее определите, как фиксируются новые требования и дополнительные работы.</p>','PUBLISHED','Этапы проекта и безопасная работа','Как этапы и критерии приёмки делают проект предсказуемее.',now()-interval '1 day',NULL,now()-interval '2 days',now()-interval '1 day'),
 (md5('seed:blog-post:draft')::uuid,md5('seed:user:admin@example.test')::uuid,md5('seed:blog-category:work')::uuid,'Черновик редакции','editorial-draft','Непубличный материал для проверки редакторского процесса.','<p>Этот материал виден только в административной части.</p>','DRAFT',NULL,NULL,NULL,NULL,now(),now()),
 (md5('seed:blog-post:scheduled')::uuid,md5('seed:user:admin@example.test')::uuid,md5('seed:blog-category:product')::uuid,'Материал по расписанию','scheduled-product-note','Статья для проверки отложенной публикации.','<p>Материал будет опубликован автоматически в указанное время.</p>','SCHEDULED',NULL,NULL,NULL,now()+interval '7 days',now(),now())
ON CONFLICT(slug) DO UPDATE SET title=EXCLUDED.title,excerpt=EXCLUDED.excerpt,content_html=EXCLUDED.content_html,status=EXCLUDED.status,published_at=EXCLUDED.published_at,scheduled_at=EXCLUDED.scheduled_at,updated_at=now();
INSERT INTO blog_post_tags(post_id,tag_id) VALUES
 (md5('seed:blog-post:brief')::uuid,md5('seed:blog-tag:brief')::uuid),(md5('seed:blog-post:portfolio')::uuid,md5('seed:blog-tag:portfolio')::uuid),(md5('seed:blog-post:safe-deal')::uuid,md5('seed:blog-tag:safe-deal')::uuid)
ON CONFLICT DO NOTHING;
