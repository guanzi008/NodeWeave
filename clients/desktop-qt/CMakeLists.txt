cmake_minimum_required(VERSION 3.21)

project(NodeWeaveDesktop VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

set(CMAKE_AUTOMOC ON)
set(CMAKE_AUTORCC ON)
set(CMAKE_AUTOUIC ON)

find_package(QT NAMES Qt6 Qt5 REQUIRED COMPONENTS Widgets Network)
find_package(Qt${QT_VERSION_MAJOR} REQUIRED COMPONENTS Widgets Network)

set(NODEWEAVE_DESKTOP_SOURCES
    src/main.cpp
    src/MainWindow.cpp
    src/MainWindow.h
    src/ControlPlaneClient.cpp
    src/ControlPlaneClient.h
    src/LocalDeviceInventory.cpp
    src/LocalDeviceInventory.h
    resources/resources.qrc
)

add_executable(nodeweave-desktop
    ${NODEWEAVE_DESKTOP_SOURCES}
)

target_link_libraries(nodeweave-desktop PRIVATE
    Qt${QT_VERSION_MAJOR}::Widgets
    Qt${QT_VERSION_MAJOR}::Network
)

target_compile_definitions(nodeweave-desktop PRIVATE
    QT_NO_KEYWORDS
)

set_target_properties(nodeweave-desktop PROPERTIES
    OUTPUT_NAME "nodeweave-desktop"
)

configure_file(
    resources/nodeweave-desktop.desktop.in
    ${CMAKE_CURRENT_BINARY_DIR}/nodeweave-desktop.desktop
    @ONLY
)

install(TARGETS nodeweave-desktop
    RUNTIME DESTINATION bin
    BUNDLE DESTINATION .
)

install(FILES
    ${CMAKE_CURRENT_BINARY_DIR}/nodeweave-desktop.desktop
    DESTINATION share/applications
)

install(FILES
    resources/nodeweave-desktop.svg
    DESTINATION share/icons/hicolor/scalable/apps
)
