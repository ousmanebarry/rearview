cmake_minimum_required(VERSION 3.26)
project(rearview_viewer VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)

if(MSVC)
  add_compile_options(/W4 /permissive- /utf-8 /Zc:__cplusplus)
  add_compile_definitions(NOMINMAX)
endif()

find_package(Qt6 REQUIRED COMPONENTS Widgets Sql Network Multimedia MultimediaWidgets)

add_executable(viewer WIN32
  src/main.cpp
  src/db.cpp
  src/main_window.cpp
  src/map_widget.cpp
  src/timeline.cpp
  src/video_view.cpp
)
target_include_directories(viewer PRIVATE src)
target_link_libraries(viewer PRIVATE Qt6::Widgets Qt6::Sql Qt6::Network Qt6::Multimedia Qt6::MultimediaWidgets)

# vcpkg copies the Qt DLLs next to the exe but not the plugins Qt loads at run time.
set(_qt_plugins "${QT6_INSTALL_PREFIX}/$<$<CONFIG:Debug>:debug/>${QT6_INSTALL_PLUGINS}")
foreach(dir platforms styles imageformats sqldrivers multimedia tls networkinformation)
  add_custom_command(TARGET viewer POST_BUILD
    COMMAND ${CMAKE_COMMAND} -E copy_directory_if_different "${_qt_plugins}/${dir}" "$<TARGET_FILE_DIR:viewer>/${dir}")
endforeach()
# DLLs only the plugins link to (qsqlite, qjpeg), which the vcpkg app-local copy misses.
set(_vcpkg_bin "${VCPKG_INSTALLED_DIR}/${VCPKG_TARGET_TRIPLET}/$<$<CONFIG:Debug>:debug/>bin")
foreach(dll sqlite3 jpeg62)
  add_custom_command(TARGET viewer POST_BUILD
    COMMAND ${CMAKE_COMMAND} -E copy_if_different "${_vcpkg_bin}/${dll}.dll" "$<TARGET_FILE_DIR:viewer>")
endforeach()
