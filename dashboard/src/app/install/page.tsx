"use client";

import { Modal } from "@components/modal/Modal";
import { useIsMd } from "@utils/responsive";
import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import SetupModal from "@/modules/setup-netbirdy-modal/SetupModal";

function InstallContent() {
  const [open, setOpen] = useState(false);
  const [mounted, setMounted] = useState(false);
  const bannerHeight = 0;
  const isMd = useIsMd();

  useEffect(() => {
    setOpen(true);
    setMounted(true);
  }, []);

  return (
    <>
      {mounted &&
        createPortal(
          <div
            className={
              "fixed top-0 left-0 right-0 z-[60] w-full pointer-events-auto"
            }
          >
            
          </div>,
          document.body,
        )}
      <Modal onOpenChange={() => null} open={open}>
        <SetupModal
          showClose={false}
          style={{ marginTop: isMd ? 0 : bannerHeight + 13 }}
        />
      </Modal>
    </>
  );
}

export default function UnauthenticatedInstallModal() {
  return (
    
      <InstallContent />
    
  );
}
