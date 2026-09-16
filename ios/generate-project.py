#!/usr/bin/env python3
"""Generate EasyUnlocker.xcodeproj/project.pbxproj deterministically.

Single iOS app target, file-system-synchronized source tree is NOT used
(objectVersion 56, explicit file lists) so the project builds with a plain
`xcodebuild -project ios/EasyUnlocker.xcodeproj -scheme EasyUnlocker
 -destination 'generic/platform=iOS Simulator' build`.
Re-run after adding/removing source files.
"""
import os
import uuid

ROOT = os.path.dirname(os.path.abspath(__file__))
APP_DIR = os.path.join(ROOT, "EasyUnlocker")
TEST_DIR = os.path.join(ROOT, "EasyUnlockerUITests")

NS = "F00DC0DE"
def uid(name: str) -> str:
    return uuid.uuid5(uuid.NAMESPACE_DNS, "easy-unlocker-ios/" + name).hex[:24].upper()

# ---- collect sources -------------------------------------------------------
ASSET_CATALOGS = []  # rel paths of *.xcassets (whole-catalog file refs)

def collect():
    groups = {}  # rel dir -> {files: [...], subdirs: set()}
    for dirpath, dirnames, filenames in os.walk(APP_DIR):
        dirnames.sort()
        rel = os.path.relpath(dirpath, APP_DIR)
        rel = "" if rel == "." else rel
        # .xcassets 是一整个资源目录，不下钻展开
        for d in list(dirnames):
            if d.endswith(".xcassets"):
                ASSET_CATALOGS.append(os.path.join(rel, d) if rel else d)
                dirnames.remove(d)
        files = sorted(f for f in filenames
                       if not f.startswith(".") and f not in ("LICENSE",))
        groups[rel] = {"files": files, "subdirs": set(dirnames)}
    return groups

GROUPS = collect()

test_srcs = []
if os.path.isdir(TEST_DIR):
    for f in sorted(os.listdir(TEST_DIR)):
        if f.endswith(".swift") and not f.startswith("."):
            test_srcs.append(f)

SRC_EXT = (".swift", ".c")
def is_source(name): return name.endswith(SRC_EXT)

# ---- emit ------------------------------------------------------------------
lines = []
def emit(s=""): lines.append(s)

objects = []
def obj(key, body):
    objects.append((key, body))

file_refs = {}   # path -> uid
build_files = {} # path -> uid

def file_ref_id(path): return uid("file/" + path)
def build_file_id(path): return uid("build/" + path)

def file_type(name):
    if name.endswith(".swift"): return "sourcecode.swift"
    if name.endswith(".c"): return "sourcecode.c.objc"
    if name.endswith(".h"): return "sourcecode.c.h"
    if name.endswith(".plist"): return "text.plist.xml"
    if name.endswith(".entitlements"): return "text.plist.entitlements"
    return "text"

# group tree under EasyUnlocker/
def group_id(path): return uid("group/" + (path or "."))

# collect every group path (incl. intermediates)
all_groups = set()
for g in GROUPS:
    parts = g.split("/") if g else []
    for i in range(len(parts) + 1):
        all_groups.add("/".join(parts[:i]))
all_groups = sorted(all_groups)

for g in all_groups:
    children = []
    info = GROUPS.get(g, {"files": [], "subdirs": set()})
    for f in info["files"]:
        p = (g + "/" + f).lstrip("/")
        children.append((file_ref_id(p), f, file_type(f)))
    subdirs = sorted(d for d in info["subdirs"]
                     if (g + "/" + d).lstrip("/") in all_groups)
    for cat in sorted(c for c in ASSET_CATALOGS
                      if os.path.dirname(c) == g or (g == "" and "/" not in c)):
        children.append((file_ref_id(cat), os.path.basename(cat), "catalog"))
    for d in subdirs:
        p = (g + "/" + d).lstrip("/")
        children.append((group_id(p), d, "group"))
    kids = "".join(
        f"\n\t\t\t\t{cid} /* {name} */," for cid, name, _ in children)
    obj(group_id(g),
        f"{group_id(g)} /* {os.path.basename(g) if g else 'EasyUnlocker'} */ = {{\n"
        f"\t\t\tisa = PBXGroup;\n\t\t\tchildren = ({kids}\n\t\t\t);\n"
        f"\t\t\tpath = {os.path.basename(g) if g else 'EasyUnlocker'};\n"
        f"\t\t\tsourceTree = \"<group>\";\n\t\t}};")

# file references + build files
src_paths = []
for g in all_groups:
    for f in GROUPS.get(g, {}).get("files", []):
        p = (g + "/" + f).lstrip("/")
        ft = file_type(f)
        obj(file_ref_id(p),
            f"{file_ref_id(p)} /* {f} */ = {{isa = PBXFileReference; "
            f"lastKnownFileType = {ft}; path = {f}; sourceTree = \"<group>\"; }};")
        if is_source(f):
            obj(build_file_id(p),
                f"{build_file_id(p)} /* {f} in Sources */ = "
                f"{{isa = PBXBuildFile; fileRef = {file_ref_id(p)} /* {f} */; }};")
            src_paths.append(p)

# asset catalogs: whole-folder refs + Resources phase entries
for cat in ASSET_CATALOGS:
    name = os.path.basename(cat)
    obj(file_ref_id(cat),
        f"{file_ref_id(cat)} /* {name} */ = {{isa = PBXFileReference; "
        f"lastKnownFileType = folder.assetcatalog; path = {name}; sourceTree = \"<group>\"; }};")
    obj(build_file_id(cat),
        f"{build_file_id(cat)} /* {name} in Resources */ = "
        f"{{isa = PBXBuildFile; fileRef = {file_ref_id(cat)} /* {name} */; }};")

# product
PRODUCT_REF = uid("product/app")
TEST_PRODUCT_REF = uid("product/uitests")
obj(PRODUCT_REF,
    f"{PRODUCT_REF} /* EasyUnlocker.app */ = {{isa = PBXFileReference; "
    "explicitFileType = wrapper.application; includeInIndex = 0; "
    "path = EasyUnlocker.app; sourceTree = BUILT_PRODUCTS_DIR; };")
obj(TEST_PRODUCT_REF,
    f"{TEST_PRODUCT_REF} /* EasyUnlockerUITests.xctest */ = {{isa = PBXFileReference; "
    "explicitFileType = wrapper.cfbundle; includeInIndex = 0; "
    "path = EasyUnlockerUITests.xctest; sourceTree = BUILT_PRODUCTS_DIR; };")

# test group + file refs
TEST_GROUP = uid("group/uitests")
test_kids = "".join(f"\n\t\t\t\t{file_ref_id('uitest/' + f)} /* {f} */,"
                    for f in test_srcs)
obj(TEST_GROUP,
    f"{TEST_GROUP} /* EasyUnlockerUITests */ = {{isa = PBXGroup; children = ({test_kids}\n"
    f"\t\t\t);\n\t\t\tpath = EasyUnlockerUITests;\n\t\t\tsourceTree = \"<group>\";\n\t\t}};")
for f in test_srcs:
    obj(file_ref_id("uitest/" + f),
        f"{file_ref_id('uitest/' + f)} /* {f} */ = {{isa = PBXFileReference; "
        f"lastKnownFileType = sourcecode.swift; path = {f}; sourceTree = \"<group>\"; }};")
    obj(build_file_id("uitest/" + f),
        f"{build_file_id('uitest/' + f)} /* {f} in Sources */ = "
        f"{{isa = PBXBuildFile; fileRef = {file_ref_id('uitest/' + f)} /* {f} */; }};")

# groups: main / products / app
MAIN_GROUP = uid("group/main")
PRODUCTS_GROUP = uid("group/products")
obj(PRODUCTS_GROUP,
    f"{PRODUCTS_GROUP} /* Products */ = {{isa = PBXGroup; children = (\n"
    f"\t\t\t\t{PRODUCT_REF} /* EasyUnlocker.app */,\n"
    f"\t\t\t\t{TEST_PRODUCT_REF} /* EasyUnlockerUITests.xctest */,\n\t\t\t);\n"
    f"\t\t\tname = Products;\n\t\t\tsourceTree = \"<group>\";\n\t\t}};")
obj(MAIN_GROUP,
    f"{MAIN_GROUP} = {{isa = PBXGroup; children = (\n"
    f"\t\t\t\t{group_id('')} /* EasyUnlocker */,\n"
    f"\t\t\t\t{TEST_GROUP} /* EasyUnlockerUITests */,\n"
    f"\t\t\t\t{PRODUCTS_GROUP} /* Products */,\n\t\t\t);\n"
    f"\t\t\tsourceTree = \"<group>\";\n\t\t}};")

# build phases
SOURCES_PHASE = uid("phase/sources")
RES_PHASE = uid("phase/resources")
FW_PHASE = uid("phase/frameworks")
src_list = "".join(f"\n\t\t\t\t{build_file_id(p)} /* {os.path.basename(p)} in Sources */,"
                   for p in sorted(src_paths))
obj(SOURCES_PHASE,
    f"{SOURCES_PHASE} /* Sources */ = {{isa = PBXSourcesBuildPhase; "
    f"buildActionMask = 2147483647; files = ({src_list}\n\t\t\t);\n"
    f"\t\t\trunOnlyForDeploymentPostprocessing = 0;\n\t\t}};")
res_list = "".join(f"\n\t\t\t\t{build_file_id(c)} /* {os.path.basename(c)} in Resources */,"
                   for c in sorted(ASSET_CATALOGS))
obj(RES_PHASE,
    f"{RES_PHASE} /* Resources */ = {{isa = PBXResourcesBuildPhase; "
    f"buildActionMask = 2147483647; files = ({res_list}\n\t\t\t);\n"
    "\t\t\trunOnlyForDeploymentPostprocessing = 0;\n\t\t};")
obj(FW_PHASE,
    f"{FW_PHASE} /* Frameworks */ = {{isa = PBXFrameworksBuildPhase; "
    "buildActionMask = 2147483647; files = (\n\t\t\t);\n"
    "\t\t\trunOnlyForDeploymentPostprocessing = 0;\n\t\t};")

# test build phases
TEST_SOURCES_PHASE = uid("phase/uitest-sources")
TEST_RES_PHASE = uid("phase/uitest-resources")
TEST_FW_PHASE = uid("phase/uitest-frameworks")
test_src_list = "".join(
    f"\n\t\t\t\t{build_file_id('uitest/' + p)} /* {p} in Sources */,"
    for p in test_srcs)
obj(TEST_SOURCES_PHASE,
    f"{TEST_SOURCES_PHASE} /* Sources */ = {{isa = PBXSourcesBuildPhase; "
    f"buildActionMask = 2147483647; files = ({test_src_list}\n\t\t\t);\n"
    f"\t\t\trunOnlyForDeploymentPostprocessing = 0;\n\t\t}};")
obj(TEST_RES_PHASE,
    f"{TEST_RES_PHASE} /* Resources */ = {{isa = PBXResourcesBuildPhase; "
    "buildActionMask = 2147483647; files = (\n\t\t\t);\n"
    "\t\t\trunOnlyForDeploymentPostprocessing = 0;\n\t\t};")
obj(TEST_FW_PHASE,
    f"{TEST_FW_PHASE} /* Frameworks */ = {{isa = PBXFrameworksBuildPhase; "
    "buildActionMask = 2147483647; files = (\n\t\t\t);\n"
    "\t\t\trunOnlyForDeploymentPostprocessing = 0;\n\t\t};")

# configurations
def cfg(name, target: bool):
    cid = uid("cfg/" + ("target/" if target else "project/") + name)
    if target:
        settings = f"""
				ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon;
				CODE_SIGN_ENTITLEMENTS = EasyUnlocker/EasyUnlocker.entitlements;
				CODE_SIGN_STYLE = Automatic;
				CURRENT_PROJECT_VERSION = 1;
				DEVELOPMENT_ASSET_PATHS = "";
				DEVELOPMENT_TEAM = "";
				ENABLE_PREVIEWS = YES;
				GENERATE_INFOPLIST_FILE = NO;
				INFOPLIST_FILE = EasyUnlocker/Info.plist;
				INFOPLIST_KEY_UIApplicationSupportsIndirectInputEvents = YES;
				INFOPLIST_KEY_UILaunchScreen_Generation = YES;
				IPHONEOS_DEPLOYMENT_TARGET = 17.0;
				LD_RUNPATH_SEARCH_PATHS = (
					"$(inherited)",
					"@executable_path/Frameworks",
				);
				MARKETING_VERSION = 1.0;
				PRODUCT_BUNDLE_IDENTIFIER = io.github.cyancity.easyunlocker;
				PRODUCT_NAME = "$(TARGET_NAME)";
				SWIFT_EMIT_LOC_STRINGS = YES;
				SWIFT_OBJC_BRIDGING_HEADER = EasyUnlocker/EasyUnlocker-Bridging-Header.h;
				HEADER_SEARCH_PATHS = "$(SRCROOT)/EasyUnlocker/Vendor/Argon2";
				SWIFT_VERSION = 5.0;
				TARGETED_DEVICE_FAMILY = 1;
"""
    else:
        common = f"""
				ALWAYS_SEARCH_USER_PATHS = NO;
				ASSETCATALOG_COMPILER_GENERATE_SWIFT_ASSET_SYMBOL_EXTENSIONS = YES;
				CLANG_ANALYZER_NONNULL = YES;
				CLANG_ANALYZER_NUMBER_OBJECT_CONVERSION = YES_AGGRESSIVE;
				CLANG_CXX_LANGUAGE_STANDARD = "gnu++20";
				CLANG_ENABLE_MODULES = YES;
				CLANG_ENABLE_OBJC_ARC = YES;
				CLANG_ENABLE_OBJC_WEAK = YES;
				CLANG_WARN_BLOCK_CAPTURE_AUTORELEASING = YES;
				CLANG_WARN_BOOL_CONVERSION = YES;
				CLANG_WARN_COMMA = YES;
				CLANG_WARN_CONSTANT_CONVERSION = YES;
				CLANG_WARN_DEPRECATED_OBJC_IMPLEMENTATIONS = YES;
				CLANG_WARN_DIRECT_OBJC_ISA_USAGE = YES_ERROR;
				CLANG_WARN_DOCUMENTATION_COMMENTS = YES;
				CLANG_WARN_EMPTY_BODY = YES;
				CLANG_WARN_ENUM_CONVERSION = YES;
				CLANG_WARN_INFINITE_RECURSION = YES;
				CLANG_WARN_INT_CONVERSION = YES;
				CLANG_WARN_NON_LITERAL_NULL_CONVERSION = YES;
				CLANG_WARN_OBJC_IMPLICIT_RETAIN_SELF = YES;
				CLANG_WARN_OBJC_LITERAL_CONVERSION = YES;
				CLANG_WARN_OBJC_ROOT_CLASS = YES_ERROR;
				CLANG_WARN_QUOTED_INCLUDE_IN_FRAMEWORK_HEADER = YES;
				CLANG_WARN_RANGE_LOOP_ANALYSIS = YES;
				CLANG_WARN_STRICT_PROTOTYPES = YES;
				CLANG_WARN_SUSPICIOUS_MOVE = YES;
				CLANG_WARN_UNGUARDED_AVAILABILITY = YES_AGGRESSIVE;
				CLANG_WARN_UNREACHABLE_CODE = YES;
				CLANG_WARN__DUPLICATE_METHOD_MATCH = YES;
				COPY_PHASE_STRIP = NO;
				DEAD_CODE_STRIPPING = YES;
				ENABLE_STRICT_OBJC_MSGSEND = YES;
				ENABLE_TESTABILITY = {"YES" if name == "Debug" else "NO"};
				ENABLE_USER_SCRIPT_SANDBOXING = YES;
				GCC_C_LANGUAGE_STANDARD = gnu17;
				GCC_DYNAMIC_NO_PIC = NO;
				GCC_NO_COMMON_BLOCKS = YES;
				GCC_WARN_64_TO_32_BIT_CONVERSION = YES;
				GCC_WARN_ABOUT_RETURN_TYPE = YES_ERROR;
				GCC_WARN_UNDECLARED_SELECTOR = YES;
				GCC_WARN_UNINITIALIZED_AUTOS = YES_AGGRESSIVE;
				GCC_WARN_UNUSED_FUNCTION = YES;
				GCC_WARN_UNUSED_VARIABLE = YES;
				IPHONEOS_DEPLOYMENT_TARGET = 17.0;
				MTL_ENABLE_DEBUG_INFO = {"INCLUDE_SOURCE" if name == "Debug" else "NO"};
				MTL_FAST_MATH = YES;
				ONLY_ACTIVE_ARCH = {"YES" if name == "Debug" else "NO"};
				SDKROOT = iphoneos;
				SWIFT_ACTIVE_COMPILATION_CONDITIONS = {"DEBUG" if name == "Debug" else '""'};
				SWIFT_OPTIMIZATION_LEVEL = {"-Onone" if name == "Debug" else "-O"};
				SWIFT_COMPILATION_MODE = {"singlefile" if name == "Debug" else "wholemodule"};
"""
        settings = common + ("""
				DEBUG_INFORMATION_FORMAT = dwarf;
				GCC_OPTIMIZATION_LEVEL = 0;
				GCC_PREPROCESSOR_DEFINITIONS = (
					"DEBUG=1",
					"$(inherited)",
				);
""" if name == "Debug" else """
				DEBUG_INFORMATION_FORMAT = "dwarf-with-dsym";
				ENABLE_NS_ASSERTIONS = NO;
				VALIDATE_PRODUCT = YES;
""")
    obj(cid,
        f"{cid} /* {name} */ = {{isa = XCBuildConfiguration; buildSettings = {{{settings}"
        f"\t\t\t}};\n\t\t\tname = {name};\n\t\t}};")
    return cid

def test_cfg(name):
    cid = uid("cfg/uitest/" + name)
    obj(cid,
        f"{cid} /* {name} */ = {{isa = XCBuildConfiguration; buildSettings = {{\n"
        f"\t\t\t\tCURRENT_PROJECT_VERSION = 1;\n"
        f"\t\t\t\tDEVELOPMENT_TEAM = \"\";\n"
        f"\t\t\t\tGENERATE_INFOPLIST_FILE = YES;\n"
        f"\t\t\t\tIPHONEOS_DEPLOYMENT_TARGET = 17.0;\n"
        f"\t\t\t\tLD_RUNPATH_SEARCH_PATHS = (\n"
        f"\t\t\t\t\t\"$(inherited)\",\n"
        f"\t\t\t\t\t\"@executable_path/Frameworks\",\n"
        f"\t\t\t\t\t\"@loader_path/Frameworks\",\n"
        f"\t\t\t\t);\n"
        f"\t\t\t\tMARKETING_VERSION = 1.0;\n"
        f"\t\t\t\tPRODUCT_BUNDLE_IDENTIFIER = io.github.cyancity.easyunlocker.UITests;\n"
        f"\t\t\t\tPRODUCT_NAME = \"$(TARGET_NAME)\";\n"
        f"\t\t\t\tSWIFT_EMIT_LOC_STRINGS = NO;\n"
        f"\t\t\t\tSWIFT_VERSION = 5.0;\n"
        f"\t\t\t\tTARGETED_DEVICE_FAMILY = 1;\n"
        f"\t\t\t\tTEST_TARGET_NAME = EasyUnlocker;\n"
        f"\t\t\t}};\n\t\t\tname = {name};\n\t\t}};")
    return cid

PROJ_CFG_LIST = uid("cfglist/project")
TGT_CFG_LIST = uid("cfglist/target")
TEST_CFG_LIST = uid("cfglist/uitest")
TARGET_ID = uid("target/app")
TEST_TARGET_ID = uid("target/uitests")
PROJECT_ID = uid("project")

dbg_p, rel_p = cfg("Debug", False), cfg("Release", False)
dbg_t, rel_t = cfg("Debug", True), cfg("Release", True)
dbg_u, rel_u = test_cfg("Debug"), test_cfg("Release")

obj(PROJ_CFG_LIST,
    f"{PROJ_CFG_LIST} /* Build configuration list for PBXProject \"EasyUnlocker\" */ = {{\n"
    f"\t\t\tisa = XCConfigurationList;\n\t\t\tbuildConfigurations = (\n"
    f"\t\t\t\t{dbg_p} /* Debug */,\n\t\t\t\t{rel_p} /* Release */,\n\t\t\t);\n"
    f"\t\t\tdefaultConfigurationIsVisible = 0;\n"
    f"\t\t\tdefaultConfigurationName = Release;\n\t\t}};")
obj(TGT_CFG_LIST,
    f"{TGT_CFG_LIST} /* Build configuration list for PBXNativeTarget \"EasyUnlocker\" */ = {{\n"
    f"\t\t\tisa = XCConfigurationList;\n\t\t\tbuildConfigurations = (\n"
    f"\t\t\t\t{dbg_t} /* Debug */,\n\t\t\t\t{rel_t} /* Release */,\n\t\t\t);\n"
    f"\t\t\tdefaultConfigurationIsVisible = 0;\n"
    f"\t\t\tdefaultConfigurationName = Release;\n\t\t}};")
obj(TEST_CFG_LIST,
    f"{TEST_CFG_LIST} /* Build configuration list for PBXNativeTarget \"EasyUnlockerUITests\" */ = {{\n"
    f"\t\t\tisa = XCConfigurationList;\n\t\t\tbuildConfigurations = (\n"
    f"\t\t\t\t{dbg_u} /* Debug */,\n\t\t\t\t{rel_u} /* Release */,\n\t\t\t);\n"
    f"\t\t\tdefaultConfigurationIsVisible = 0;\n"
    f"\t\t\tdefaultConfigurationName = Release;\n\t\t}};")

# test target depends on app target
TEST_PROXY_ID = uid("proxy/uitest")
TEST_DEP_ID = uid("dep/uitest")
obj(TEST_PROXY_ID,
    f"{TEST_PROXY_ID} /* PBXContainerItemProxy */ = {{isa = PBXContainerItemProxy;\n"
    f"\t\t\tcontainerPortal = {PROJECT_ID} /* Project object */;\n"
    f"\t\t\tproxyType = 1;\n"
    f"\t\t\tremoteGlobalIDString = {TARGET_ID};\n"
    f"\t\t\tremoteInfo = EasyUnlocker;\n\t\t}};")
obj(TEST_DEP_ID,
    f"{TEST_DEP_ID} /* PBXTargetDependency */ = {{isa = PBXTargetDependency;\n"
    f"\t\t\ttarget = {TARGET_ID} /* EasyUnlocker */;\n"
    f"\t\t\ttargetProxy = {TEST_PROXY_ID} /* PBXContainerItemProxy */;\n\t\t}};")

obj(TARGET_ID,
    f"{TARGET_ID} /* EasyUnlocker */ = {{isa = PBXNativeTarget;\n"
    f"\t\t\tbuildConfigurationList = {TGT_CFG_LIST} /* Build configuration list for PBXNativeTarget \"EasyUnlocker\" */;\n"
    f"\t\t\tbuildPhases = (\n"
    f"\t\t\t\t{SOURCES_PHASE} /* Sources */,\n"
    f"\t\t\t\t{FW_PHASE} /* Frameworks */,\n"
    f"\t\t\t\t{RES_PHASE} /* Resources */,\n"
    f"\t\t\t);\n"
    f"\t\t\tbuildRules = (\n\t\t\t);\n"
    f"\t\t\tdependencies = (\n\t\t\t);\n"
    f"\t\t\tname = EasyUnlocker;\n"
    f"\t\t\tproductName = EasyUnlocker;\n"
    f"\t\t\tproductReference = {PRODUCT_REF} /* EasyUnlocker.app */;\n"
    f"\t\t\tproductType = \"com.apple.product-type.application\";\n\t\t}};")

obj(TEST_TARGET_ID,
    f"{TEST_TARGET_ID} /* EasyUnlockerUITests */ = {{isa = PBXNativeTarget;\n"
    f"\t\t\tbuildConfigurationList = {TEST_CFG_LIST} /* Build configuration list for PBXNativeTarget \"EasyUnlockerUITests\" */;\n"
    f"\t\t\tbuildPhases = (\n"
    f"\t\t\t\t{TEST_SOURCES_PHASE} /* Sources */,\n"
    f"\t\t\t\t{TEST_FW_PHASE} /* Frameworks */,\n"
    f"\t\t\t\t{TEST_RES_PHASE} /* Resources */,\n"
    f"\t\t\t);\n"
    f"\t\t\tbuildRules = (\n\t\t\t);\n"
    f"\t\t\tdependencies = (\n\t\t\t\t{TEST_DEP_ID} /* PBXTargetDependency */,\n\t\t\t);\n"
    f"\t\t\tname = EasyUnlockerUITests;\n"
    f"\t\t\tproductName = EasyUnlockerUITests;\n"
    f"\t\t\tproductReference = {TEST_PRODUCT_REF} /* EasyUnlockerUITests.xctest */;\n"
    f"\t\t\tproductType = \"com.apple.product-type.bundle.ui-testing\";\n\t\t}};")

obj(PROJECT_ID,
    f"{PROJECT_ID} /* Project object */ = {{isa = PBXProject;\n"
    f"\t\t\tattributes = {{\n"
    f"\t\t\t\tBuildIndependentTargetsInParallel = 1;\n"
    f"\t\t\t\tLastSwiftUpdateCheck = 2600;\n"
    f"\t\t\t\tLastUpgradeCheck = 2600;\n"
    f"\t\t\t\tTargetAttributes = {{\n"
    f"\t\t\t\t\t{TARGET_ID} = {{\n"
    f"\t\t\t\t\t\tCreatedOnToolsVersion = 26.0;\n"
    f"\t\t\t\t\t}};\n"
    f"\t\t\t\t\t{TEST_TARGET_ID} = {{\n"
    f"\t\t\t\t\t\tCreatedOnToolsVersion = 26.0;\n"
    f"\t\t\t\t\t\tTestTargetID = {TARGET_ID};\n"
    f"\t\t\t\t\t}};\n"
    f"\t\t\t\t}};\n"
    f"\t\t\t}};\n"
    f"\t\t\tbuildConfigurationList = {PROJ_CFG_LIST} /* Build configuration list for PBXProject \"EasyUnlocker\" */;\n"
    f"\t\t\tcompatibilityVersion = \"Xcode 14.0\";\n"
    f"\t\t\tdevelopmentRegion = en;\n"
    f"\t\t\thasScannedForEncodings = 0;\n"
    f"\t\t\tknownRegions = (\n\t\t\t\ten,\n\t\t\t\tBase,\n\t\t\t);\n"
    f"\t\t\tmainGroup = {MAIN_GROUP};\n"
    f"\t\t\tproductRefGroup = {PRODUCTS_GROUP} /* Products */;\n"
    f"\t\t\tprojectDirPath = \"\";\n"
    f"\t\t\tprojectRoot = \"\";\n"
    f"\t\t\ttargets = (\n\t\t\t\t{TARGET_ID} /* EasyUnlocker */,\n"
    f"\t\t\t\t{TEST_TARGET_ID} /* EasyUnlockerUITests */,\n\t\t\t);\n\t\t}};")

# ---- assemble --------------------------------------------------------------
emit("// !$*UTF8*$!")
emit("{")
emit("\tarchiveVersion = 1;")
emit("\tclasses = {")
emit("\t};")
emit("\tobjectVersion = 56;")
emit("\tobjects = {")
emit("")
# deterministic order by section then key
section_of = {
    "PBXBuildFile": [], "PBXFileReference": [], "PBXGroup": [],
    "PBXSourcesBuildPhase": [], "PBXResourcesBuildPhase": [],
    "PBXFrameworksBuildPhase": [], "PBXNativeTarget": [], "PBXProject": [],
    "PBXContainerItemProxy": [], "PBXTargetDependency": [],
    "XCBuildConfiguration": [], "XCConfigurationList": [],
}
for k, body in objects:
    isa = body.split("isa = ")[1].split(";")[0].strip()
    section_of[isa].append((k, body))

for sec in ["PBXBuildFile", "PBXFileReference", "PBXContainerItemProxy",
            "PBXFrameworksBuildPhase",
            "PBXGroup", "PBXNativeTarget", "PBXProject",
            "PBXResourcesBuildPhase", "PBXSourcesBuildPhase",
            "PBXTargetDependency",
            "XCBuildConfiguration", "XCConfigurationList"]:
    entries = sorted(section_of[sec])
    emit(f"/* Begin {sec} section */")
    for _, body in entries:
        emit("\t\t" + body)
    emit(f"/* End {sec} section */")
    emit("")
emit("\t};")
emit(f"\trootObject = {PROJECT_ID} /* Project object */;")
emit("}")

proj_dir = os.path.join(ROOT, "EasyUnlocker.xcodeproj")
os.makedirs(proj_dir, exist_ok=True)
out = os.path.join(proj_dir, "project.pbxproj")
with open(out, "w") as fh:
    fh.write("\n".join(lines) + "\n")

# shared scheme
scheme_dir = os.path.join(proj_dir, "xcshareddata", "xcschemes")
os.makedirs(scheme_dir, exist_ok=True)
scheme = f"""<?xml version="1.0" encoding="UTF-8"?>
<Scheme
   LastUpgradeVersion = "2600"
   version = "1.7">
   <BuildAction
      parallelizeBuildables = "YES"
      buildImplicitDependencies = "YES"
      buildArchitectures = "Automatic">
      <BuildActionEntries>
         <BuildActionEntry
            buildForTesting = "YES"
            buildForRunning = "YES"
            buildForProfiling = "YES"
            buildForArchiving = "YES"
            buildForAnalyzing = "YES">
            <BuildableReference
               BuildableIdentifier = "primary"
               BlueprintIdentifier = "{TARGET_ID}"
               BuildableName = "EasyUnlocker.app"
               BlueprintName = "EasyUnlocker"
               ReferencedContainer = "container:EasyUnlocker.xcodeproj">
            </BuildableReference>
         </BuildActionEntry>
         <BuildActionEntry
            buildForTesting = "YES"
            buildForRunning = "NO"
            buildForProfiling = "NO"
            buildForArchiving = "NO"
            buildForAnalyzing = "NO">
            <BuildableReference
               BuildableIdentifier = "primary"
               BlueprintIdentifier = "{TEST_TARGET_ID}"
               BuildableName = "EasyUnlockerUITests.xctest"
               BlueprintName = "EasyUnlockerUITests"
               ReferencedContainer = "container:EasyUnlocker.xcodeproj">
            </BuildableReference>
         </BuildActionEntry>
      </BuildActionEntries>
   </BuildAction>
   <TestAction
      buildConfiguration = "Debug"
      selectedDebuggerIdentifier = "Xcode.DebuggerFoundation.Debugger.LLDB"
      selectedLauncherIdentifier = "Xcode.DebuggerFoundation.Launcher.LLDB"
      shouldUseLaunchSchemeArgsEnv = "YES"
      shouldAutocreateTestPlan = "YES">
      <Testables>
         <TestableReference
            skipped = "NO">
            <BuildableReference
               BuildableIdentifier = "primary"
               BlueprintIdentifier = "{TEST_TARGET_ID}"
               BuildableName = "EasyUnlockerUITests.xctest"
               BlueprintName = "EasyUnlockerUITests"
               ReferencedContainer = "container:EasyUnlocker.xcodeproj">
            </BuildableReference>
         </TestableReference>
      </Testables>
   </TestAction>
   <LaunchAction
      buildConfiguration = "Debug"
      selectedDebuggerIdentifier = "Xcode.DebuggerFoundation.Debugger.LLDB"
      selectedLauncherIdentifier = "Xcode.DebuggerFoundation.Launcher.LLDB"
      launchStyle = "0"
      useCustomWorkingDirectory = "NO"
      ignoresPersistentStateOnLaunch = "NO"
      debugDocumentVersioning = "YES"
      debugServiceExtension = "internal"
      allowLocationSimulation = "YES">
      <BuildableProductRunnable
         runnableDebuggingMode = "0">
         <BuildableReference
            BuildableIdentifier = "primary"
            BlueprintIdentifier = "{TARGET_ID}"
            BuildableName = "EasyUnlocker.app"
            BlueprintName = "EasyUnlocker"
            ReferencedContainer = "container:EasyUnlocker.xcodeproj">
         </BuildableReference>
      </BuildableProductRunnable>
   </LaunchAction>
   <ProfileAction
      buildConfiguration = "Release"
      shouldUseLaunchSchemeArgsEnv = "YES"
      savedToolIdentifier = ""
      useCustomWorkingDirectory = "NO"
      debugDocumentVersioning = "YES">
      <BuildableProductRunnable
         runnableDebuggingMode = "0">
         <BuildableReference
            BuildableIdentifier = "primary"
            BlueprintIdentifier = "{TARGET_ID}"
            BuildableName = "EasyUnlocker.app"
            BlueprintName = "EasyUnlocker"
            ReferencedContainer = "container:EasyUnlocker.xcodeproj">
         </BuildableReference>
      </BuildableProductRunnable>
   </ProfileAction>
   <AnalyzeAction
      buildConfiguration = "Debug">
   </AnalyzeAction>
   <ArchiveAction
      buildConfiguration = "Release"
      revealArchiveInOrganizer = "YES">
   </ArchiveAction>
</Scheme>
"""
with open(os.path.join(scheme_dir, "EasyUnlocker.xcscheme"), "w") as fh:
    fh.write(scheme)
print(f"wrote {out} ({len(src_paths)} source files)")
