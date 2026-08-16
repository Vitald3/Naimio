"use client";
import { useParams } from "next/navigation";
import AdminContentDetail from "../../content-detail";
export default function Page(){const params=useParams<{id:string}>();return <AdminContentDetail endpoint="vacancies" kind="VACANCY" id={params.id}/>}
