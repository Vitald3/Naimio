"use client";

import { useState } from "react";
import Breadcrumbs from "../../../breadcrumbs";
import ServiceEditor from "../service-editor";

const serviceCreationCapabilities = {
  types: ["PROFESSIONAL_SERVICE", "CONSULTATION", "EDUCATION", "MENTORING"],
  educationFields: ["duration_minutes", "sessions_count", "audience_type", "group_size_max"],
};

export default function NewService(){
  const [createdID,setCreatedID]=useState("");
  async function publish(){
    const response=await fetch(`/api/v1/me/services/${createdID}/publish`,{method:"POST",credentials:"same-origin"});
    if(response.ok)location.assign(`/services/${createdID}`);
    else window.dispatchEvent(new CustomEvent("naimio:toast",{detail:{tone:"error",message:"Не удалось опубликовать: проверьте профиль и данные предложения"}}));
  }
  return <main data-service-types={serviceCreationCapabilities.types.join(",")} data-education-fields={serviceCreationCapabilities.educationFields.join(",")}><Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Мои услуги",href:"/dashboard/services"},{label:"Новое предложение"}]}/><header className="page-heading"><div><p className="eyebrow">Каталог услуг</p><h1>Новое предложение</h1><p className="lead">Оформите услугу, консультацию, обучение или наставничество в единой карточке Naimio.</p></div></header><ServiceEditor onSaved={setCreatedID}/>{createdID?<div className="publish-panel"><div><strong>Черновик готов к публикации</strong><p>Проверьте данные — после публикации предложение появится в каталоге.</p></div><button type="button" onClick={()=>void publish()}>Опубликовать</button></div>:null}</main>
}
