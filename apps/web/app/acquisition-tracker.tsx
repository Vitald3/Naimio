"use client";
import { useEffect } from "react";
import { track } from "./analytics";
import { STAFF_BASE_PATH } from "./admin-path";
const excluded=["/dashboard","/messages","/settings","/notifications","/favorites","/invite",STAFF_BASE_PATH];
export default function AcquisitionTracker(){useEffect(()=>{if(!excluded.some(value=>location.pathname.startsWith(value)))track("LANDING_VIEW")},[]);return null}
