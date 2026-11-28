SOURCES = {
    "atiga": {
        "atiga-text": {"title": "ASEAN Trade in Goods Agreement (Cha-am, 26 February 2009), Chapter 3 Articles 26 to 33", "url": "https://asean.org/asean-trade-in-goods-agreement-atiga/", "sha256": "46c6d3367e1453c907e8b0091efda16316266a5bc139bbe1489f4686d228c0f6", "kind": "primary_treaty_text"},
        "atiga-annex3": {"title": "ATIGA Annex 3, Product Specific Rules, HS 2022 (endorsed by the 36th AFTA Council)", "url": "https://asean.org/wp-content/uploads/2025/09/ATIGA-PSR-HS-2022-endorsed-AFTAC36.xlsx", "sha256": "bb4d6a6fb8a8e9052cfb5790053add3bd66e9631a2ee09b7333076ddb24dba07", "kind": "primary_rule_table"},
        "atiga-annex6": {"title": "ATIGA Annex 6, Implementing Guidelines for Partial Cumulation under Article 30.2", "url": "https://asean.org/asean-trade-in-goods-agreement-atiga/", "sha256": "a38dc731d39383ee80e50accb504bbbb6ae48157c2d5225a3f570c180e195362", "kind": "primary_treaty_text"},
        "atiga-handbook": {"title": "Singapore Customs, Handbook on Rules of Origin for Preferential Certificates of Origin (Dec 2025)", "url": "https://isomer-user-content.by.gov.sg/174/fc3ecb0b-d701-4d8f-9881-9d99f90f35a2/Handbook_on_ROO_for_PCO_Dec2025__clean_.pdf", "sha256": "6f9db34eb9923abb0831f30e70f1f67478c861dea63fc7f591cb9577e724da85", "kind": "authority_guidance"},
        "atiga-miti": {"title": "MITI Malaysia, Rules of Origin worked examples (AFTA/ATIGA)", "url": "https://www.miti.gov.my/index.php/pages/view/2071", "kind": "authority_guidance"},
    },
    "akfta": {
        "akfta-annex3": {"title": "AKFTA Annex 3, Rules of Origin, Rules 1 to 10", "sha256": "9412966eadfb699b355f28b36fadb61a3ae63685a605dd872c962c2d4e62ff60", "kind": "primary_treaty_text", "note": "Archived copy named in D949; the file is not committed."},
        "akfta-appendix2": {"title": "AKFTA Appendix 2 Product Specific Rules in HS 2022 (AKFTA Implementing Committee, to be implemented 1 May 2026)", "url": "https://ftadjbc.info/download/akfta-product-specific-rules-in-hs-2022-2/", "sha256": "eae76c720fcfcb68126e211989a08cb5d2549b1959a01dc3063e26296347ce93", "kind": "primary_rule_table"},
        "akfta-miti-guide": {"title": "MITI Malaysia, Guide to the ASEAN-Korea FTA, Part 5: Guide to Rules of Origin (worked examples)", "url": "https://fta.miti.gov.my/miti-fta/resources/auto%20download%20images/558b9ea8df8ce.pdf", "sha256": "8e3a30575fcd545a29a39711f02176ef8fde4f53a5ff7f4380bb7e162b8ff6fc", "kind": "authority_guidance"},
    },
    "acfta": {
        "acfta-annex1": {"title": "ACFTA Upgrading Protocol (2015), Annex 1 Rules of Origin, Articles 1 to 9", "sha256": "98fe8eb5d66dfdcf02b9e2a41b138ee248636bcf497934771dea7df053ad4e63", "kind": "primary_treaty_text", "note": "Archived copy named in D949; the file is not committed."},
        "acfta-attb": {"title": "ACFTA Attachment B, Product Specific Rules, HS 2017 (Enterprise Singapore copy)", "url": "https://www.enterprisesg.gov.sg/-/media/esg/files/non-financial-assistance/for-companies/free-trade-agreements/ASEAN-China-FTA/upgrade-protocol/Attachment-B-PSR.pdf", "sha256": "2559405c67a73c0b9473959742b7148ae22c47ddcab05f004a471fb6c9379899", "kind": "primary_rule_table"},
        "acfta-jetro": {"title": "JETRO, ASEAN-China Free Trade Agreement (ACFTA) guide, August 2020 revision, slides 17 to 20", "url": "https://www.jetro.go.jp/ext_images/theme/wto-fta/asean_fta/pdf/acfta_202008rev.pdf", "sha256": "787fd882268193d0c992e46aaa4be4f4b76cc8e70aafd8fcc06a42fb5300d67c", "kind": "authority_guidance"},
    },
    "rcep": {
        "rcep-ch3": {"title": "RCEP Agreement, Chapter 3 Rules of Origin, Articles 3.2 to 3.7", "url": "https://rcepsec.org/rules-of-origin/", "kind": "primary_treaty_text", "note": "The local RCEP agreement PDF is image-only and the single fetch of the chapter text returned 404, so article numbers follow the engine's pinned citations and the ABF guide; the clauses were not re-read in this lane."},
        "rcep-annex3a": {"title": "RCEP Annex 3A Product-Specific Rules, HS 2022 (RCEP Secretariat)", "url": "https://rcepsec.org/wp-content/uploads/2024/07/RCEP-PSR-in-HS-2022-adopted-by-RJC-30June2022-corrected20Aug2022.pdf", "sha256": "e474dd17238c1c3fb126d9762d10de849ffd8c061a3f42fa596f3833a3536fd6", "kind": "primary_rule_table"},
        "abf-rcep-guide": {"title": "Australian Border Force, Regional Comprehensive Economic Partnership (RCEP) Rules of Origin guide (18 December 2024)", "url": "https://www.abf.gov.au/free-trade-agreements/files/RCEP-rules-of-origin.pdf", "kind": "authority_guidance"},
    },
}

PARENT_SOURCE = {
    "atiga": {"miti": "atiga-miti", "sgc": "atiga-handbook"},
    "akfta": {"miti": "akfta-miti-guide"},
    "acfta": {"jetro": "acfta-jetro"},
}
